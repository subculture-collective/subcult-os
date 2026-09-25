package app

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	atprotocol "git.subcult.tv/PatrickFanella/subcult-os/internal/atproto"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Greeting(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "world"
	}
	return "Hello, " + name + "!"
}

type App struct {
	config         Config
	db             *pgxpool.Pool
	payments       paymentProvider
	media          mediaStorage
	mediaErr       error
	discovery      discoveryPolicy
	identity       *identityProtector
	identityErr    error
	atprotoOAuth   *atprotocol.OAuthClient
	atprotoErr     error
	atprotoStore   *atprotocol.OAuthStore
	atprotoFlow    atprotoLinkFlow
	atprotoFlowErr error
	lexiconCatalog *atprotocol.LexiconCatalog
	lexiconErr     error
	recordFetcher  atprotocol.RecordFetcher
	// consentCheckOverride lets tests substitute checkSendPermission with a
	// fake that returns an arbitrary (including non-sentinel, infra-shaped)
	// error, without touching the database. Left nil in production, where
	// processEmailDeliveries always calls the real checkSendPermission.
	consentCheckOverride func(ctx context.Context, workspaceID, channel, recipient, purpose string) error
	mux                  *http.ServeMux
	loginMu              sync.Mutex
	loginAttempts        map[string]loginAttempt
}

func New(config Config, db *pgxpool.Pool) *App {
	media, mediaErr := newMediaStorage(config)
	identity, identityErr := newIdentityProtector(config.IdentityProtectionKey, config.IdentityProtectionKeyPrevious, config.SessionSecret)
	var atprotoOAuth *atprotocol.OAuthClient
	var atprotoErr error
	if config.ATProtoOAuthEnabled {
		atprotoOAuth, atprotoErr = atprotocol.NewOAuthClient(config.atprotoOAuthSettings())
	}
	var atprotoFlow atprotoLinkFlow
	var atprotoFlowErr error
	var atprotoStore *atprotocol.OAuthStore
	if config.ATProtoOAuthEnabled && atprotoErr == nil {
		if db == nil {
			atprotoFlowErr = errors.New("AT OAuth flow requires a database")
		} else {
			store, err := atprotocol.NewOAuthStore(db, config.IdentityProtectionKey, config.IdentityProtectionKeyPrevious, config.SessionSecret)
			if err != nil {
				atprotoFlowErr = err
			} else {
				atprotoStore = store
				atprotoFlow, atprotoFlowErr = atprotoOAuth.NewOAuthFlow(store)
			}
		}
	}
	// The admitted Lexicon catalog is loaded from disk on a best-effort
	// basis. D5/ADR 0007 has not been accepted and no production deployment
	// currently ships contracts/lexicons alongside the binary, so a missing
	// directory is expected outside test/dev environments; only the
	// public-preview endpoint (which validates against the catalog) fails
	// when it is unavailable, not application startup.
	var lexiconCatalog *atprotocol.LexiconCatalog
	var lexiconErr error
	if config.LexiconContractDir != "" {
		lexiconCatalog, lexiconErr = atprotocol.LoadLexiconCatalog(config.LexiconContractDir)
	} else {
		lexiconCatalog, lexiconErr = atprotocol.LoadEmbeddedLexiconCatalog()
	}
	a := &App{config: config, db: db, payments: newStripePaymentProvider(config.StripeSecretKey), media: media, mediaErr: mediaErr, discovery: newDiscoveryPolicy(), identity: identity, identityErr: identityErr, atprotoOAuth: atprotoOAuth, atprotoErr: atprotoErr, atprotoStore: atprotoStore, atprotoFlow: atprotoFlow, atprotoFlowErr: atprotoFlowErr, lexiconCatalog: lexiconCatalog, lexiconErr: lexiconErr, recordFetcher: atprotocol.NewIdentityRecordFetcher(), mux: http.NewServeMux(), loginAttempts: map[string]loginAttempt{}}
	a.routes()
	return a
}

func (a *App) Handler() http.Handler {
	return a.requestLogger(a.cors(a.originGuard(a.operatorSession(a.mux))))
}

type operatorPersonKey struct{}

// Operator namespaces require authentication before resource authorization.
// 401 lets web/native clients refresh; 403 remains an actual permission denial.
func (a *App) operatorSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		original := r
		path := r.URL.Path
		if path == "/api/workspaces" || strings.HasPrefix(path, "/api/workspaces/") || strings.HasPrefix(path, "/api/events/") {
			person, ok := a.requirePersonID(r)
			if !ok {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			r = r.WithContext(context.WithValue(r.Context(), operatorPersonKey{}, person))
		}
		next.ServeHTTP(w, r)
		// Preserve the server-owned route template for the outer safe logger.
		original.Pattern = r.Pattern
	})
}

func (a *App) routes() {
	a.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	a.mux.HandleFunc("GET /api/ready", a.handleReady)
	a.mux.HandleFunc("POST /api/auth/signup", a.handleSignup)
	a.mux.HandleFunc("POST /api/auth/verify-email", a.handleVerifyEmail)
	a.mux.HandleFunc("POST /api/auth/request-verification", a.handleRequestVerification)
	a.mux.HandleFunc("POST /api/auth/login", a.handleLogin)
	a.mux.HandleFunc("POST /api/auth/refresh", a.handleRefreshSession)
	a.mux.HandleFunc("POST /api/auth/logout", a.handleLogout)
	a.mux.HandleFunc("POST /api/auth/logout-all", a.handleLogoutAll)
	a.mux.HandleFunc("POST /api/auth/recovery/request", a.handleRequestRecovery)
	a.mux.HandleFunc("POST /api/auth/recovery/complete", a.handleCompleteRecovery)
	a.mux.HandleFunc("POST /api/mobile/auth/signup", a.handleSignup)
	a.mux.HandleFunc("POST /api/mobile/auth/verify-email", a.handleVerifyEmail)
	a.mux.HandleFunc("POST /api/mobile/auth/request-verification", a.handleRequestVerification)
	a.mux.HandleFunc("POST /api/mobile/auth/login", a.handleLogin)
	a.mux.HandleFunc("POST /api/mobile/auth/refresh", a.handleRefreshSession)
	a.mux.HandleFunc("POST /api/mobile/auth/logout", a.handleLogout)
	a.mux.HandleFunc("POST /api/mobile/auth/logout-all", a.handleLogoutAll)
	a.mux.HandleFunc("POST /api/mobile/auth/recovery/request", a.handleRequestRecovery)
	a.mux.HandleFunc("POST /api/mobile/auth/recovery/complete", a.handleCompleteRecovery)
	a.mux.HandleFunc("GET /api/me", a.handleMe)
	a.mux.HandleFunc("GET /api/me/participant-portal", a.handleGetParticipantPortal)
	a.mux.HandleFunc("GET /api/debug/mobile-auth", a.handleMobileAuthDebug)
	a.mux.HandleFunc("GET /api/dev/email-outbox", a.handleDevEmailOutbox)
	a.mux.HandleFunc("GET /api/v1/auth/atproto/client-metadata", a.handleATProtoClientMetadata)
	a.mux.HandleFunc("GET /api/v1/auth/atproto/jwks", a.handleATProtoJWKS)
	a.mux.HandleFunc("POST /api/v1/auth/atproto/start", a.handleATProtoStart)
	a.mux.HandleFunc("GET /api/v1/auth/atproto/callback", a.handleATProtoCallback)
	a.mux.HandleFunc("GET /api/v1/auth/atproto/links", a.handleATProtoLinks)
	a.mux.HandleFunc("DELETE /api/v1/auth/atproto/links/{did}", a.handleATProtoUnlink)
	a.mux.HandleFunc("POST /api/workspaces", a.handleCreateWorkspace)
	a.mux.HandleFunc("GET /api/workspaces/current", a.handleCurrentWorkspace)
	a.mux.HandleFunc("GET /api/workspaces/{workspaceID}", a.handleGetWorkspace)
	a.mux.HandleFunc("GET /api/workspaces/{workspaceID}/event-templates", a.handleListEventTemplates)
	a.mux.HandleFunc("POST /api/workspaces/{workspaceID}/event-templates", a.handleCreateEventTemplate)
	a.mux.HandleFunc("PATCH /api/workspaces/{workspaceID}/event-templates/{templateID}", a.handleUpdateEventTemplate)
	a.mux.HandleFunc("DELETE /api/workspaces/{workspaceID}/event-templates/{templateID}", a.handleDeleteEventTemplate)
	a.mux.HandleFunc("GET /api/workspaces/{workspaceID}/contacts", a.handleListContacts)
	a.mux.HandleFunc("POST /api/workspaces/{workspaceID}/contacts", a.handleCreateContact)
	a.mux.HandleFunc("PATCH /api/workspaces/{workspaceID}/contacts/{contactID}", a.handleUpdateContact)
	a.mux.HandleFunc("GET /api/workspaces/{workspaceID}/commitments", a.handleListWorkspaceCommitments)
	a.mux.HandleFunc("POST /api/workspaces/{workspaceID}/commitments", a.handleCreateCommitment)
	a.mux.HandleFunc("PATCH /api/workspaces/{workspaceID}/commitments/{commitmentID}", a.handleUpdateCommitment)
	a.mux.HandleFunc("GET /api/workspaces/{workspaceID}/reminders", a.handleListWorkspaceReminders)
	a.mux.HandleFunc("POST /api/workspaces/{workspaceID}/reminders/sweep", a.handleSweepWorkspaceReminders)
	a.mux.HandleFunc("GET /api/workspaces/{workspaceID}/archives", a.handleListWorkspaceArchives)
	a.mux.HandleFunc("POST /api/workspaces/{workspaceID}/invitations", a.handleCreateInvitation)
	a.mux.HandleFunc("POST /api/invitations/{token}/accept", a.handleAcceptInvitation)
	a.mux.HandleFunc("DELETE /api/workspaces/{workspaceID}/members/{memberID}", a.handleRemoveMember)
	a.mux.HandleFunc("PATCH /api/workspaces/{workspaceID}/members/{memberID}", a.handleUpdateWorkspaceMember)
	a.mux.HandleFunc("POST /api/workspaces/{workspaceID}/members/{memberID}/revoke", a.handleRevokeWorkspaceMember)
	a.mux.HandleFunc("GET /api/workspaces/{workspaceID}/delegations", a.handleListDelegations)
	a.mux.HandleFunc("POST /api/workspaces/{workspaceID}/delegations", a.handleCreateDelegation)
	a.mux.HandleFunc("POST /api/workspaces/{workspaceID}/delegations/{delegationID}/revoke", a.handleRevokeDelegation)
	a.mux.HandleFunc("POST /api/workspaces/{workspaceID}/consent-grants", a.handleCreateConsentGrant)
	a.mux.HandleFunc("GET /api/workspaces/{workspaceID}/consent-grants", a.handleListConsentGrants)
	a.mux.HandleFunc("POST /api/workspaces/{workspaceID}/consent-grants/{grantID}/withdraw", a.handleOperatorWithdrawConsentGrant)
	a.mux.HandleFunc("POST /api/workspaces/{workspaceID}/announcements", a.handleCreateAnnouncement)
	a.mux.HandleFunc("GET /api/workspaces/{workspaceID}/announcements", a.handleListAnnouncements)
	a.mux.HandleFunc("GET /api/workspaces/{workspaceID}/announcements/{announcementID}", a.handleGetAnnouncement)
	a.mux.HandleFunc("GET /api/workspaces/{workspaceID}/announcements/{announcementID}/preview", a.handlePreviewAnnouncement)
	a.mux.HandleFunc("POST /api/workspaces/{workspaceID}/announcements/{announcementID}/schedule", a.handleScheduleAnnouncement)
	a.mux.HandleFunc("POST /api/workspaces/{workspaceID}/announcements/{announcementID}/cancel", a.handleCancelAnnouncement)
	a.mux.HandleFunc("POST /api/workspaces/{workspaceID}/cultural-imports/preview", a.handleCreateCulturalImportPreview)
	a.mux.HandleFunc("GET /api/workspaces/{workspaceID}/cultural-imports/{importID}", a.handleGetCulturalImportPreview)
	a.mux.HandleFunc("POST /api/workspaces/{workspaceID}/cultural-imports/apply", a.handleApplyCulturalImport)
	a.mux.HandleFunc("POST /api/workspaces/{workspaceID}/cultural-import-actions/{actionID}/rollback", a.handleRollbackCulturalImportAction)
	a.mux.HandleFunc("GET /api/workspaces/{workspaceID}/events", a.handleListEvents)
	a.mux.HandleFunc("POST /api/workspaces/{workspaceID}/events", a.handleCreateEvent)
	a.mux.HandleFunc("GET /api/events/{eventID}", a.handleGetEvent)
	a.mux.HandleFunc("POST /api/events/{eventID}/apply-template", a.handleApplyEventTemplate)
	a.mux.HandleFunc("PATCH /api/events/{eventID}", a.handleUpdateEvent)
	a.mux.HandleFunc("POST /api/events/{eventID}/image", a.handleUploadEventImage)
	a.mux.HandleFunc("POST /api/events/{eventID}/publish", a.handlePublishEvent)
	a.mux.HandleFunc("POST /api/events/{eventID}/test-ticket", a.handleCreateTestTicket)
	a.mux.HandleFunc("GET /api/workspaces/{workspaceID}/profiles", a.handleListCulturalProfiles)
	a.mux.HandleFunc("POST /api/workspaces/{workspaceID}/profiles", a.handleCreateCulturalProfile)
	a.mux.HandleFunc("PATCH /api/workspaces/{workspaceID}/profiles/{profileID}", a.handleUpdateCulturalProfile)
	a.mux.HandleFunc("GET /api/workspaces/{workspaceID}/places", a.handleListCulturalPlaces)
	a.mux.HandleFunc("POST /api/workspaces/{workspaceID}/places", a.handleCreateCulturalPlace)
	a.mux.HandleFunc("PATCH /api/workspaces/{workspaceID}/places/{placeID}", a.handleUpdateCulturalPlace)
	a.mux.HandleFunc("GET /api/events/{eventID}/occurrences", a.handleListEventOccurrences)
	a.mux.HandleFunc("POST /api/events/{eventID}/occurrences", a.handleCreateEventOccurrence)
	a.mux.HandleFunc("PATCH /api/events/{eventID}/occurrences/{occurrenceID}", a.handleUpdateEventOccurrence)
	a.mux.HandleFunc("POST /api/events/{eventID}/occurrences/{occurrenceID}/credits", a.handleAttachOccurrenceCredit)
	a.mux.HandleFunc("DELETE /api/events/{eventID}/occurrences/{occurrenceID}/credits/{profileID}", a.handleDetachOccurrenceCredit)
	a.mux.HandleFunc("GET /api/events/{eventID}/occurrences/{occurrenceID}/public-preview", a.handleOccurrencePublicPreview)
	a.mux.HandleFunc("POST /api/events/{eventID}/public-links/preview", a.handlePreviewEventPublicLink)
	a.mux.HandleFunc("GET /api/events/{eventID}/public-links", a.handleListEventPublicLinks)
	a.mux.HandleFunc("POST /api/events/{eventID}/public-links", a.handleAttachEventPublicLink)
	a.mux.HandleFunc("DELETE /api/events/{eventID}/public-links/{linkID}", a.handleDetachEventPublicLink)
	a.mux.HandleFunc("POST /api/events/{eventID}/public-links/{linkID}/refresh", a.handleRefreshEventPublicLink)
	a.mux.HandleFunc("GET /api/events/{eventID}/commitments", a.handleListEventCommitments)
	a.mux.HandleFunc("GET /api/events/{eventID}/reminders", a.handleListEventReminders)
	a.mux.HandleFunc("GET /api/events/{eventID}/notifications", a.handleListEventNotifications)
	a.mux.HandleFunc("GET /api/events/{eventID}/roles", a.handleListEventRoles)
	a.mux.HandleFunc("POST /api/events/{eventID}/roles", a.handleCreateEventRole)
	a.mux.HandleFunc("PATCH /api/events/{eventID}/roles/{roleID}", a.handleUpdateEventRole)
	a.mux.HandleFunc("GET /api/events/{eventID}/staffing", a.handleListEventStaffing)
	a.mux.HandleFunc("POST /api/events/{eventID}/staffing", a.handleCreateEventStaffing)
	a.mux.HandleFunc("PATCH /api/events/{eventID}/staffing/{staffingID}", a.handleUpdateEventStaffing)
	a.mux.HandleFunc("GET /api/events/{eventID}/role-applications", a.handleListEventRoleApplications)
	a.mux.HandleFunc("GET /api/events/{eventID}/participants", a.handleListEventParticipants)
	a.mux.HandleFunc("PATCH /api/events/{eventID}/role-applications/{applicationID}", a.handleReviewEventRoleApplication)
	a.mux.HandleFunc("POST /api/events/{eventID}/end-of-night", a.handleEndOfNight)
	a.mux.HandleFunc("GET /api/events/{eventID}/report", a.handleGetReport)
	a.mux.HandleFunc("GET /api/events/{eventID}/exports/settlement.csv", a.handleGetSettlementCSV)
	a.mux.HandleFunc("GET /api/events/{eventID}/exports/settlement.md", a.handleSettlementMarkdown)
	a.mux.HandleFunc("GET /api/events/{eventID}/exports/settlement-print.html", a.handleSettlementPrint)
	a.mux.HandleFunc("GET /api/events/{eventID}/archive", a.handleGetArchive)
	a.mux.HandleFunc("GET /api/events/{eventID}/public-archive-items", a.handleListPublicArchiveItems)
	a.mux.HandleFunc("POST /api/events/{eventID}/public-archive-items", a.handleCreatePublicArchiveItem)
	a.mux.HandleFunc("POST /api/events/{eventID}/public-archive-items/{itemID}/unavailable", a.handleUnavailablePublicArchiveItem)
	a.mux.HandleFunc("POST /api/events/{eventID}/public-archive-items/{itemID}/correct", a.handleCorrectPublicArchiveItem)
	a.mux.HandleFunc("POST /api/events/{eventID}/archive/notes", a.handleCreateArchiveNote)
	a.mux.HandleFunc("POST /api/events/{eventID}/archive/seed-draft", a.handleSeedDraftFromArchive)
	a.mux.HandleFunc("GET /api/events/{eventID}/settlement", a.handleGetSettlement)
	a.mux.HandleFunc("POST /api/events/{eventID}/settlement/finalize", a.handleFinalizeSettlement)
	a.mux.HandleFunc("POST /api/events/{eventID}/settlement/adjustments", a.handleCreateSettlementAdjustment)
	a.mux.HandleFunc("GET /api/public/discovery/occurrences", a.handleListPublicDiscoveryOccurrences)
	a.mux.HandleFunc("GET /api/public/discovery/occurrences/{uri...}", a.handleGetPublicDiscoveryOccurrence)
	a.mux.HandleFunc("GET /api/public/events", a.handleListPublicEvents)
	a.mux.HandleFunc("GET /api/public/events/{slug}", a.handlePublicEvent)
	a.mux.HandleFunc("GET /api/public/events/{slug}/roles", a.handleListPublicEventRoles)
	a.mux.HandleFunc("POST /api/public/events/{slug}/role-applications", a.handleSubmitPublicRoleApplication)
	a.mux.HandleFunc("POST /api/public/events/{slug}/reservations", a.handleReserveTicket)
	a.mux.HandleFunc("POST /api/public/events/{slug}/paid-reservations", a.handleCreatePaidReservation)
	a.mux.HandleFunc("POST /api/public/consent/{token}/confirm", a.handleConfirmConsentGrant)
	a.mux.HandleFunc("POST /api/public/consent/{token}/withdraw", a.handleWithdrawConsentGrant)
	a.mux.HandleFunc("POST /api/stripe/webhook", a.handleStripeWebhook)
	a.mux.HandleFunc("POST /api/resend/webhook", a.handleResendWebhook)
	a.mux.HandleFunc("GET /api/tickets/{code}", a.handleGetTicket)
	a.mux.HandleFunc("GET /api/events/{eventID}/door/tickets", a.handleDoorTicketSearch)
	a.mux.HandleFunc("POST /api/events/{eventID}/door/check-ins", a.handleDoorCheckIn)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (a *App) handleReady(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.db.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (a *App) originGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestNeedsOriginCheck(r) && !a.allowedOrigin(r) {
			writeError(w, http.StatusForbidden, "origin not allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" && a.allowedOrigin(r) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, "+authTokenHeader+", "+refreshTokenHeader)
			w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Expose-Headers", authTokenHeader+", "+refreshTokenHeader)
			w.Header().Add("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(body)
}

func (a *App) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		// Paths include bearer ticket/invitation values and linked DIDs. Log
		// only the server-owned route template, never user-supplied URLs. A
		// denied or unmatched request may not have reached the mux at all.
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		log.Printf("method=%s route=%q status=%d duration=%s", r.Method, route, recorder.status, time.Since(started).Round(time.Millisecond))
	})
}

func requestNeedsOriginCheck(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return false
	}
	_, accessCookieErr := r.Cookie(authCookieName)
	_, refreshCookieErr := r.Cookie(refreshCookieName)
	if accessCookieErr != nil && refreshCookieErr != nil && strings.TrimSpace(r.Header.Get(authTokenHeader)) == "" && strings.TrimSpace(r.Header.Get(refreshTokenHeader)) == "" && strings.TrimSpace(r.Header.Get("Authorization")) == "" {
		return false
	}
	return r.Header.Get("Origin") != ""
}

func (a *App) allowedOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	if strings.EqualFold(parsed.Host, r.Host) {
		return true
	}
	if a.isDevelopment() && isLocalDevOrigin(parsed) {
		return true
	}
	publicWebURL := strings.TrimSpace(a.config.PublicWebURL)
	if publicWebURL == "" {
		return false
	}
	publicParsed, err := url.Parse(publicWebURL)
	if err != nil || publicParsed.Scheme == "" || publicParsed.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Scheme, publicParsed.Scheme) && strings.EqualFold(parsed.Host, publicParsed.Host)
}

func (a *App) isDevelopment() bool {
	return strings.EqualFold(strings.TrimSpace(a.config.AppEnv), "development") || strings.TrimSpace(a.config.AppEnv) == ""
}

func isLocalDevOrigin(origin *url.URL) bool {
	if origin.Scheme != "http" && origin.Scheme != "https" {
		return false
	}
	hostname := strings.ToLower(origin.Hostname())
	return hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1"
}
