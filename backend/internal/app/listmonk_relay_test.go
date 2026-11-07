package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func korkTestBinding() listmonkBinding {
	listURL, sender, purpose, disclosure := "https://lists.example.test", "Signal Collective", "Show announcements", "Once you confirm, your address is added."
	return listmonkBinding{ID: "b", ListID: 7, KorkListURL: &listURL, KorkSender: &sender, KorkPurpose: &purpose, KorkDisclosure: &disclosure}
}

func TestValidateKorkReceipt(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	binding := korkTestBinding()
	// The exact shape Kork writes (internal/server/signup.go signupConsent).
	valid := `{"v":1,"list":"listmonk:https://lists.example.test#7","sender":"Signal Collective","page":"https://kork.example.test/signal","purpose":"Show announcements","disclosure":"Once you confirm, your address is added.","at":"2026-11-06T12:00:00Z"}`
	if _, code := validateKorkReceipt(binding, json.RawMessage(valid), now); code != "" {
		t.Fatalf("valid receipt refused: %s", code)
	}
	cases := map[string]string{
		strings.Replace(valid, "Signal Collective", "Other", 1):    "receipt_wrong_sender",
		strings.Replace(valid, "Show announcements", "Merch", 1):   "receipt_wrong_purpose",
		strings.Replace(valid, "#7", "#8", 1):                      "receipt_wrong_list",
		strings.Replace(valid, "is added.", "is shared.", 1):       "receipt_wrong_disclosure",
		strings.Replace(valid, `"v":1`, `"v":2`, 1):                "receipt_invalid",
		strings.Replace(valid, `"v":1,`, `"v":1,"extra":true,`, 1): "receipt_invalid",
		valid + `{}`: "receipt_invalid",
		`[]`:         "receipt_invalid",
		strings.Replace(valid, "2026-11-06T12:00:00Z", "2026-04-01T12:00:00Z", 1): "receipt_expired",
		strings.Replace(valid, "2026-11-06T12:00:00Z", "2026-11-07T13:00:00Z", 1): "receipt_expired",
	}
	for raw, want := range cases {
		if _, code := validateKorkReceipt(binding, json.RawMessage(raw), now); code != want {
			t.Fatalf("%s: got %q, want %q", raw, code, want)
		}
	}
	if _, code := validateKorkReceipt(listmonkBinding{ListID: 7}, json.RawMessage(valid), now); code != "binding_accepts_no_kork_receipts" {
		t.Fatalf("binding without policy accepted a receipt: %q", code)
	}
}

func TestClassifyListmonkSend(t *testing.T) {
	cases := []struct {
		result listmonkResult
		status string
	}{
		{listmonkResult{status: 200, sent: true}, listmonkStatusAccepted},
		{listmonkResult{sent: false, err: errors.New("dial")}, listmonkStatusPending},
		{listmonkResult{sent: true, err: errors.New("timeout")}, listmonkStatusUncertain},
		{listmonkResult{status: http.StatusTooManyRequests, sent: true}, listmonkStatusPending},
		{listmonkResult{status: http.StatusServiceUnavailable, sent: true}, listmonkStatusPending},
		{listmonkResult{status: http.StatusBadGateway, sent: true}, listmonkStatusUncertain},
		{listmonkResult{status: http.StatusForbidden, sent: true}, listmonkStatusFailed},
		{listmonkResult{status: http.StatusBadRequest, sent: true}, listmonkStatusRejected},
	}
	for _, tc := range cases {
		if status, _ := classifyListmonkSend(tc.result); status != tc.status {
			t.Fatalf("%+v: got %s, want %s", tc.result, status, tc.status)
		}
	}
	// Before /api/tx nothing was sent, so a transport error or 5xx retries.
	if status, _ := classifyListmonkPreSend(listmonkResult{sent: true, err: errors.New("timeout")}); status != listmonkStatusPending {
		t.Fatalf("pre-send timeout = %s", status)
	}
	if status, _ := classifyListmonkPreSend(listmonkResult{status: 500, sent: true}); status != listmonkStatusPending {
		t.Fatalf("pre-send 500 = %s", status)
	}
}

func TestListmonkRelayConfigValidation(t *testing.T) {
	base := Config{ListmonkURL: "https://lists.example.test", ListmonkAPIUser: "relay", ListmonkAPIToken: "token", AppEnv: "production"}
	if problems := base.listmonkRelayProblems(); len(problems) != 0 {
		t.Fatalf("valid config: %v", problems)
	}
	if problems := (Config{}).listmonkRelayProblems(); len(problems) != 0 || (Config{}).listmonkRelayConfigured() {
		t.Fatal("empty config must disable the relay without problems")
	}
	for name, config := range map[string]Config{
		"http in production": {ListmonkURL: "http://lists.example.test", ListmonkAPIUser: "relay", ListmonkAPIToken: "token", AppEnv: "production"},
		"partial":            {ListmonkURL: "https://lists.example.test"},
		"user characters":    {ListmonkURL: "https://lists.example.test", ListmonkAPIUser: "relay user", ListmonkAPIToken: "token"},
		"credentials in URL": {ListmonkURL: "https://u:p@lists.example.test", ListmonkAPIUser: "relay", ListmonkAPIToken: "token"},
	} {
		if len(config.listmonkRelayProblems()) == 0 {
			t.Fatalf("%s accepted", name)
		}
	}
	dev := base
	dev.AppEnv, dev.ListmonkURL = "development", "http://listmonk:9000"
	if problems := dev.listmonkRelayProblems(); len(problems) != 0 {
		t.Fatalf("http outside production: %v", problems)
	}
}
