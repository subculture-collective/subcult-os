package app

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

type backlineContractFixtures struct {
	Publish          map[string]any `json:"publish"`
	Cancel           map[string]any `json:"cancel"`
	Receipt          map[string]any `json:"receipt"`
	ForbiddenKeys    []string       `json:"forbiddenKeys"`
	SignatureVectors []struct {
		Scope     string `json:"scope"`
		Secret    string `json:"secret"`
		Timestamp int64  `json:"timestamp"`
		Body      string `json:"body"`
		Signature string `json:"signature"`
	} `json:"signatureVectors"`
}

func loadBacklineFixtures(t *testing.T) backlineContractFixtures {
	t.Helper()
	raw, err := os.ReadFile("../../../contracts/backline-handoff.v1.fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var fx backlineContractFixtures
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	return fx
}

// jsonKeyPaths lists every object key path, so a payload can be compared to
// the contract example field by field.
func jsonKeyPaths(value any, prefix string, out map[string]bool) {
	object, ok := value.(map[string]any)
	if !ok {
		return
	}
	for key, child := range object {
		out[prefix+key] = true
		jsonKeyPaths(child, prefix+key+".", out)
	}
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func TestBacklineSignatureVectors(t *testing.T) {
	fx := loadBacklineFixtures(t)
	if len(fx.SignatureVectors) != 2 {
		t.Fatalf("expected handoff and receipt vectors, got %d", len(fx.SignatureVectors))
	}
	for _, vector := range fx.SignatureVectors {
		got := backlineSignature(vector.Secret, vector.Scope, vector.Timestamp, []byte(vector.Body))
		if got != vector.Signature {
			t.Fatalf("%s signature = %s, want %s", vector.Scope, got, vector.Signature)
		}
		header := http.Header{}
		header.Set(backlineTimestampHeader, strconv.FormatInt(vector.Timestamp, 10))
		header.Set(backlineSignatureHeader, vector.Signature)
		at := time.Unix(vector.Timestamp, 0)
		if err := verifyBacklineSignature(vector.Secret, vector.Scope, header, []byte(vector.Body), at.Add(time.Minute)); err != nil {
			t.Fatalf("%s vector must verify: %v", vector.Scope, err)
		}
		other := map[string]string{"handoff": "receipt", "receipt": "handoff"}[vector.Scope]
		for name, check := range map[string]func() error{
			"tampered body": func() error {
				return verifyBacklineSignature(vector.Secret, vector.Scope, header, []byte(vector.Body+" "), at)
			},
			"wrong secret": func() error {
				return verifyBacklineSignature(vector.Secret+"x", vector.Scope, header, []byte(vector.Body), at)
			},
			"other scope": func() error { return verifyBacklineSignature(vector.Secret, other, header, []byte(vector.Body), at) },
			"stale timestamp": func() error {
				return verifyBacklineSignature(vector.Secret, vector.Scope, header, []byte(vector.Body), at.Add(6*time.Minute))
			},
			"future timestamp": func() error {
				return verifyBacklineSignature(vector.Secret, vector.Scope, header, []byte(vector.Body), at.Add(-6*time.Minute))
			},
		} {
			if check() == nil {
				t.Fatalf("%s %s must be rejected", vector.Scope, name)
			}
		}
	}
}

func TestBacklinePayloadMatchesContractAndCarriesNoAudience(t *testing.T) {
	fx := loadBacklineFixtures(t)
	content := fx.Publish["content"].(map[string]any)
	revision := announcementContentRevision(content["subject"].(string), content["body"].(string))
	if revision != fx.Publish["contentRevision"] {
		t.Fatalf("content revision %s does not match the contract example %s", revision, fx.Publish["contentRevision"])
	}
	payload := backlineHandoffPayload{
		Version: backlineHandoffVersion, Kind: backlineHandoffKind, Operation: handoffOperationPublish,
		OccurrenceKey: backlineOccurrenceKey("a", "b"), ContentRevision: revision,
		Binding:   backlineBindingRef{ID: "b", ServerID: "112233445566778899", ChannelID: "998877665544332211"},
		DeliverAt: "2026-11-08T18:00:00Z", ExpiresAt: "2026-11-09T18:00:00Z", MentionPolicy: backlineMentionPolicyNone,
		Content: &backlineContent{Subject: "s", Body: "b"}, SentAt: "2026-11-07T12:00:00Z",
	}
	for _, name := range []string{"publish", "cancel"} {
		example := map[string]map[string]any{"publish": fx.Publish, "cancel": fx.Cancel}[name]
		if name == "cancel" {
			payload.Operation, payload.Content = handoffOperationCancel, nil
		}
		encoded, _ := json.Marshal(payload)
		var decoded any
		_ = json.Unmarshal(encoded, &decoded)
		got, want := map[string]bool{}, map[string]bool{}
		jsonKeyPaths(decoded, "", got)
		jsonKeyPaths(example, "", want)
		if strings.Join(sortedKeys(got), ",") != strings.Join(sortedKeys(want), ",") {
			t.Fatalf("%s payload fields %v differ from contract example %v", name, sortedKeys(got), sortedKeys(want))
		}
		for path := range got {
			leaf := path[strings.LastIndex(path, ".")+1:]
			for _, forbidden := range fx.ForbiddenKeys {
				if strings.EqualFold(leaf, forbidden) {
					t.Fatalf("%s payload exposes forbidden field %s", name, path)
				}
			}
		}
	}
}

func TestSendBacklineHandoffClassifiesReceiverResults(t *testing.T) {
	const secret = "unit-test-backline-secret-0123456789"
	payload := backlineHandoffPayload{Version: 1, Kind: backlineHandoffKind, Operation: handoffOperationPublish, OccurrenceKey: "key-1"}
	cases := []struct {
		name, wantStatus, wantCode string
		handler                    http.HandlerFunc
	}{
		{"accepted", handoffStatusAccepted, "", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"version":1,"occurrenceKey":"key-1","status":"accepted","receiverReference":"job-1"}`))
		}},
		{"duplicate", handoffStatusAccepted, "", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"version":1,"occurrenceKey":"key-1","status":"duplicate"}`))
		}},
		{"wrong key ack", handoffStatusUncertain, "receiver_ack_invalid", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"version":1,"occurrenceKey":"other","status":"accepted"}`))
		}},
		{"cancel status for publish", handoffStatusUncertain, "receiver_ack_invalid", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"version":1,"occurrenceKey":"key-1","status":"cancelled"}`))
		}},
		{"definitive rejection", handoffStatusRejected, "binding_revoked", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"binding_revoked"}`))
		}},
		{"unexplained rejection", handoffStatusRejected, "receiver_http_403", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}},
		{"not processed", handoffStatusPending, "receiver_http_503", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}},
		{"rate limited", handoffStatusPending, "receiver_http_429", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}},
		{"server error after send", handoffStatusUncertain, "receiver_http_500", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}},
		{"timeout", handoffStatusUncertain, "receiver_no_response", func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var verified bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				verified = verifyBacklineSignature(secret, backlineSignatureScopeHandoff, r.Header, body, time.Now()) == nil &&
					r.Header.Get("Idempotency-Key") == "key-1"
				tc.handler(w, r)
			}))
			defer server.Close()
			client := server.Client()
			client.Timeout = 300 * time.Millisecond
			status, _, _, code := sendBacklineHandoff(t.Context(), Config{BacklineHandoffURL: server.URL, BacklineHandoffSecret: secret}, client, payload)
			if status != tc.wantStatus || code != tc.wantCode {
				t.Fatalf("got %s/%s, want %s/%s", status, code, tc.wantStatus, tc.wantCode)
			}
			if !verified {
				t.Fatal("receiver could not verify the signed request")
			}
		})
	}

	t.Run("unreachable receiver retries", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		address := listener.Addr().String()
		_ = listener.Close()
		status, _, _, code := sendBacklineHandoff(t.Context(), Config{BacklineHandoffURL: "http://" + address, BacklineHandoffSecret: secret}, http.DefaultClient, payload)
		if status != handoffStatusPending || code != "receiver_unreachable" {
			t.Fatalf("got %s/%s, want pending/receiver_unreachable", status, code)
		}
	})
}

func TestBacklineRetryDelayIsBounded(t *testing.T) {
	if backlineRetryDelay(1) != 30*time.Second || backlineRetryDelay(2) != time.Minute || backlineRetryDelay(20) != 30*time.Minute {
		t.Fatalf("unexpected retry delays: %v %v %v", backlineRetryDelay(1), backlineRetryDelay(2), backlineRetryDelay(20))
	}
}

func TestBacklineHandoffConfigValidation(t *testing.T) {
	secret := strings.Repeat("s", 32)
	cases := []struct {
		name   string
		config Config
		valid  bool
	}{
		{"disabled", Config{}, true},
		{"https", Config{BacklineHandoffURL: "https://backline.example.test/os", BacklineHandoffSecret: secret}, true},
		{"http outside production", Config{BacklineHandoffURL: "http://127.0.0.1:9/os", BacklineHandoffSecret: secret}, true},
		{"http in production", Config{AppEnv: "production", BacklineHandoffURL: "http://backline.example.test/os", BacklineHandoffSecret: secret}, false},
		{"url without secret", Config{BacklineHandoffURL: "https://backline.example.test/os"}, false},
		{"secret without url", Config{BacklineHandoffSecret: secret}, false},
		{"short secret", Config{BacklineHandoffURL: "https://backline.example.test/os", BacklineHandoffSecret: "short"}, false},
		{"credentials in url", Config{BacklineHandoffURL: "https://user:pass@backline.example.test/os", BacklineHandoffSecret: secret}, false},
	}
	for _, tc := range cases {
		problems := tc.config.backlineHandoffProblems()
		if (len(problems) == 0) != tc.valid {
			t.Fatalf("%s: problems=%v, want valid=%v", tc.name, problems, tc.valid)
		}
	}
	if (Config{BacklineHandoffURL: "https://backline.example.test"}).backlineHandoffConfigured() {
		t.Fatal("a URL without a secret must not enable the adapter")
	}
}
