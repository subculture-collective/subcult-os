package app

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"unicode"

	atprotocol "git.subcult.tv/PatrickFanella/subcult-os/internal/atproto"
	mailprovider "git.subcult.tv/PatrickFanella/subcult-os/internal/mail"
)

type Config struct {
	AppEnv                        string
	DatabaseURL                   string
	SessionSecret                 string
	IdentityProtectionKey         string
	IdentityProtectionKeyPrevious string
	PublicWebURL                  string
	ATProtoOAuthEnabled           bool
	ATProtoOAuthClientID          string
	ATProtoOAuthCallback          string
	ATProtoOAuthJWKSURL           string
	ATProtoOAuthKey               string
	ATProtoOAuthKeyID             string
	ATProtoOAuthKeyPrevious       string
	ATProtoOAuthKeyIDPrevious     string
	atProtoOAuthEnabledRaw        string
	Addr                          string
	StripeSecretKey               string
	StripeWebhookSecret           string
	MediaS3Endpoint               string
	MediaS3AccessKey              string
	MediaS3SecretKey              string
	MediaS3Bucket                 string
	MediaS3Region                 string
	MediaPublicBaseURL            string
	MailDeliveryEnabled           bool
	mailDeliveryEnabledRaw        string
	MailProvider                  string
	BrevoAPIKey                   string
	BrevoWebhookToken             string
	ResendAPIKey                  string
	ResendWebhookSecret           string
	MailFrom                      string
	MailReplyTo                   string
	LexiconContractDir            string
	ATProjectionEnabled           bool
	atProjectionEnabledRaw        string
	ATProjectionSourceURL         string
	// ExternalTicketAllowedHosts is an explicit hostname allowlist for the
	// operator-configured external purchase-link handoff. An empty list denies
	// every handoff; it never selects or contacts a ticket provider.
	ExternalTicketAllowedHosts    []string
	externalTicketAllowedHostsRaw string
	// AnnouncementUnitCostCents is the observed per-recipient cost basis
	// used only to compute an announcement's estimated_cost_cents
	// (count * unit cost) for preview and reporting; it is never sent to
	// the provider and never gates whether an announcement dispatches.
	// Default 0 means "unknown cost", not "free": see
	// docs/development/announcements.md.
	AnnouncementUnitCostCents    int
	announcementUnitCostCentsRaw string
	// BacklineHandoffURL and BacklineHandoffSecret enable the opt-in Backline
	// announcement handoff (docs/development/backline-handoff.md). Both empty
	// by default; nothing is sent or accepted unless both are set.
	BacklineHandoffURL    string
	BacklineHandoffSecret string
	// Native app link verification. Each list is empty unless configured, and
	// the matching /.well-known document is served only when it is set.
	MobileAppleAppIDs             []string
	MobileAndroidPackage          string
	MobileAndroidCertFingerprints []string
}

func LoadConfig() Config {
	atProtoOAuthEnabledRaw := env("ATPROTO_OAUTH_ENABLED", "false")
	mailDeliveryEnabledRaw := env("MAIL_DELIVERY_ENABLED", "false")
	atProjectionEnabledRaw := env("AT_PROJECTION_ENABLED", "false")
	announcementUnitCostCentsRaw := env("ANNOUNCEMENT_UNIT_COST_CENTS", "0")
	externalTicketAllowedHostsRaw := env("EXTERNAL_TICKET_ALLOWED_HOSTS", "")
	announcementUnitCostCents, _ := strconv.Atoi(announcementUnitCostCentsRaw)
	return Config{
		AnnouncementUnitCostCents:     announcementUnitCostCents,
		BacklineHandoffURL:            strings.TrimSpace(env("BACKLINE_HANDOFF_URL", "")),
		BacklineHandoffSecret:         env("BACKLINE_HANDOFF_SECRET", ""),
		announcementUnitCostCentsRaw:  announcementUnitCostCentsRaw,
		ATProjectionEnabled:           parseEnvBool(atProjectionEnabledRaw),
		atProjectionEnabledRaw:        atProjectionEnabledRaw,
		ATProjectionSourceURL:         env("AT_PROJECTION_SOURCE_URL", ""),
		ExternalTicketAllowedHosts:    parseExternalTicketAllowedHosts(externalTicketAllowedHostsRaw),
		externalTicketAllowedHostsRaw: externalTicketAllowedHostsRaw,
		MailDeliveryEnabled:           parseEnvBool(mailDeliveryEnabledRaw),
		mailDeliveryEnabledRaw:        mailDeliveryEnabledRaw,
		MailProvider:                  env("MAIL_PROVIDER", "resend"), BrevoAPIKey: env("BREVO_API_KEY", ""), BrevoWebhookToken: env("BREVO_WEBHOOK_TOKEN", ""),
		ResendAPIKey:                  env("RESEND_API_KEY", ""),
		ResendWebhookSecret:           env("RESEND_WEBHOOK_SECRET", ""),
		MailFrom:                      env("MAIL_FROM", ""),
		MailReplyTo:                   env("MAIL_REPLY_TO", ""),
		AppEnv:                        env("APP_ENV", "development"),
		DatabaseURL:                   env("DATABASE_URL", ""),
		SessionSecret:                 env("SESSION_SECRET", "dev-session-secret-change-me"),
		IdentityProtectionKey:         env("IDENTITY_PROTECTION_KEY", ""),
		IdentityProtectionKeyPrevious: env("IDENTITY_PROTECTION_KEY_PREVIOUS", ""),
		PublicWebURL:                  env("PUBLIC_WEB_URL", "http://localhost:5173"),
		ATProtoOAuthEnabled:           parseEnvBool(atProtoOAuthEnabledRaw),
		ATProtoOAuthClientID:          env("ATPROTO_OAUTH_CLIENT_ID", ""),
		ATProtoOAuthCallback:          env("ATPROTO_OAUTH_CALLBACK_URL", ""),
		ATProtoOAuthJWKSURL:           env("ATPROTO_OAUTH_JWKS_URL", ""),
		ATProtoOAuthKey:               env("ATPROTO_OAUTH_CLIENT_PRIVATE_KEY", ""),
		ATProtoOAuthKeyID:             env("ATPROTO_OAUTH_CLIENT_KEY_ID", "subcults-1"),
		ATProtoOAuthKeyPrevious:       env("ATPROTO_OAUTH_CLIENT_PRIVATE_KEY_PREVIOUS", ""),
		ATProtoOAuthKeyIDPrevious:     env("ATPROTO_OAUTH_CLIENT_KEY_ID_PREVIOUS", ""),
		atProtoOAuthEnabledRaw:        atProtoOAuthEnabledRaw,
		Addr:                          env("API_ADDR", ":8080"),
		StripeSecretKey:               env("STRIPE_SECRET_KEY", ""),
		StripeWebhookSecret:           env("STRIPE_WEBHOOK_SECRET", ""),
		MediaS3Endpoint:               env("MEDIA_S3_ENDPOINT", ""),
		MediaS3AccessKey:              env("MEDIA_S3_ACCESS_KEY", ""),
		MediaS3SecretKey:              env("MEDIA_S3_SECRET_KEY", ""),
		MediaS3Bucket:                 env("MEDIA_S3_BUCKET", ""),
		MediaS3Region:                 env("MEDIA_S3_REGION", "us-east-1"),
		MediaPublicBaseURL:            env("MEDIA_PUBLIC_BASE_URL", ""),
		LexiconContractDir:            env("LEXICON_CONTRACT_DIR", ""),
		MobileAppleAppIDs:             parseCommaList(env("MOBILE_APPLE_APP_IDS", ""), strings.TrimSpace),
		MobileAndroidPackage:          strings.TrimSpace(env("MOBILE_ANDROID_PACKAGE", "")),
		MobileAndroidCertFingerprints: parseCommaList(env("MOBILE_ANDROID_CERT_SHA256", ""), strings.ToUpper),
	}
}

func parseExternalTicketAllowedHosts(raw string) []string {
	var hosts []string
	for _, value := range strings.Split(raw, ",") {
		if host := strings.ToLower(strings.TrimSpace(value)); host != "" {
			hosts = append(hosts, host)
		}
	}
	return hosts
}

func (c Config) Validate() error {
	var problems []string
	if strings.IndexFunc(c.externalTicketAllowedHostsRaw, unicode.IsControl) >= 0 {
		problems = append(problems, "EXTERNAL_TICKET_ALLOWED_HOSTS must not contain control characters")
	}
	for _, host := range c.ExternalTicketAllowedHosts {
		if strings.IndexFunc(host, unicode.IsControl) >= 0 {
			problems = append(problems, "EXTERNAL_TICKET_ALLOWED_HOSTS must not contain control characters")
			break
		}
		parsed, err := url.Parse("https://" + strings.TrimSpace(host))
		if err != nil || parsed.Hostname() == "" || parsed.Hostname() != strings.TrimSpace(host) || parsed.User != nil || parsed.Path != "" {
			problems = append(problems, "EXTERNAL_TICKET_ALLOWED_HOSTS must contain hostnames only")
			break
		}
	}
	if c.ResendWebhookSecret != "" && !mailprovider.ValidWebhookSecret(c.ResendWebhookSecret) {
		problems = append(problems, "RESEND_WEBHOOK_SECRET must be a valid signing secret")
	}
	if raw := c.mailDeliveryEnabledRaw; raw != "" {
		if _, err := strconv.ParseBool(raw); err != nil {
			problems = append(problems, "MAIL_DELIVERY_ENABLED must be a boolean")
		}
	}
	if c.MailProvider != "" && c.MailProvider != "resend" && c.MailProvider != "brevo" {
		problems = append(problems, "MAIL_PROVIDER must be resend or brevo")
	}
	if c.BrevoWebhookToken != "" && !mailprovider.ValidBrevoWebhookToken(c.BrevoWebhookToken) {
		problems = append(problems, "BREVO_WEBHOOK_TOKEN must be a 32..256 character token")
	}
	if c.MailDeliveryEnabled {
		if c.mailProvider() == "brevo" {
			if !mailprovider.ValidBrevoWebhookToken(c.BrevoWebhookToken) {
				problems = append(problems, "BREVO_WEBHOOK_TOKEN is required for mail delivery")
			}
			if _, err := mailprovider.NewBrevo(c.BrevoAPIKey); err != nil {
				problems = append(problems, "BREVO_API_KEY is required for mail delivery")
			}
		} else {
			if !mailprovider.ValidWebhookSecret(c.ResendWebhookSecret) {
				problems = append(problems, "RESEND_WEBHOOK_SECRET is required for mail delivery")
			}
			if _, err := mailprovider.NewResend(c.ResendAPIKey); err != nil {
				problems = append(problems, "RESEND_API_KEY is required for mail delivery")
			}
		}

		if !mailprovider.ValidAddress(c.MailFrom) {
			problems = append(problems, "MAIL_FROM must be a valid sender address")
		}
		if c.MailReplyTo != "" && !mailprovider.ValidAddress(c.MailReplyTo) {
			problems = append(problems, "MAIL_REPLY_TO must be a valid address")
		}
	}
	if raw := strings.TrimSpace(c.atProtoOAuthEnabledRaw); raw != "" {
		if _, err := strconv.ParseBool(raw); err != nil {
			problems = append(problems, "ATPROTO_OAUTH_ENABLED must be a boolean")
		}
	}
	if raw := strings.TrimSpace(c.atProjectionEnabledRaw); raw != "" {
		if _, err := strconv.ParseBool(raw); err != nil {
			problems = append(problems, "AT_PROJECTION_ENABLED must be a boolean")
		}
	}
	if c.ATProjectionEnabled && strings.TrimSpace(c.ATProjectionSourceURL) == "" {
		problems = append(problems, "AT_PROJECTION_SOURCE_URL is required when AT_PROJECTION_ENABLED is true")
	}
	if raw := strings.TrimSpace(c.announcementUnitCostCentsRaw); raw != "" {
		if value, err := strconv.Atoi(raw); err != nil || value < 0 {
			problems = append(problems, "ANNOUNCEMENT_UNIT_COST_CENTS must be a non-negative integer")
		}
	}
	problems = append(problems, c.backlineHandoffProblems()...)
	problems = append(problems, c.appLinkProblems()...)
	if strings.TrimSpace(c.Addr) == "" {
		problems = append(problems, "API_ADDR is required")
	}
	if strings.EqualFold(strings.TrimSpace(c.AppEnv), "production") {
		if strings.TrimSpace(c.DatabaseURL) == "" {
			problems = append(problems, "DATABASE_URL is required in production")
		}
		if strings.TrimSpace(c.PublicWebURL) == "" {
			problems = append(problems, "PUBLIC_WEB_URL is required in production")
		}
		secret := strings.TrimSpace(c.SessionSecret)
		if secret == "" || secret == "dev-session-secret-change-me" || len(secret) < 24 {
			problems = append(problems, "SESSION_SECRET must be a non-default value with at least 24 characters in production")
		}
		if _, err := decodeIdentityProtectionKey(c.IdentityProtectionKey); err != nil {
			problems = append(problems, "IDENTITY_PROTECTION_KEY must be base64 for exactly 32 bytes in production")
		}
		if previous := strings.TrimSpace(c.IdentityProtectionKeyPrevious); previous != "" {
			if _, err := decodeIdentityProtectionKey(previous); err != nil {
				problems = append(problems, "IDENTITY_PROTECTION_KEY_PREVIOUS must be base64 for exactly 32 bytes when set")
			}
		}
		stripeSecretKey := strings.TrimSpace(c.StripeSecretKey)
		stripeWebhookSecret := strings.TrimSpace(c.StripeWebhookSecret)
		if stripeSecretKey == "" && stripeWebhookSecret != "" {
			problems = append(problems, "STRIPE_SECRET_KEY is required when STRIPE_WEBHOOK_SECRET is set in production")
		}
		if stripeSecretKey != "" && stripeWebhookSecret == "" {
			problems = append(problems, "STRIPE_WEBHOOK_SECRET is required when STRIPE_SECRET_KEY is set in production")
		}
	}
	if c.ATProtoOAuthEnabled {
		if err := atprotocol.ValidateOAuthClientSettings(c.atprotoOAuthSettings()); err != nil {
			problems = append(problems, err.Error())
		}
		publicURL, err := url.Parse(strings.TrimSpace(c.PublicWebURL))
		if err != nil || publicURL.Scheme != "https" || publicURL.Hostname() == "" || publicURL.User != nil || publicURL.RawQuery != "" || publicURL.Fragment != "" || (publicURL.Path != "" && publicURL.Path != "/") {
			problems = append(problems, "PUBLIC_WEB_URL must be an HTTPS origin when AT OAuth is enabled")
		} else if clientURL, parseErr := url.Parse(strings.TrimSpace(c.ATProtoOAuthClientID)); parseErr == nil && !strings.EqualFold(publicURL.Host, clientURL.Host) {
			problems = append(problems, "PUBLIC_WEB_URL and ATPROTO_OAUTH_CLIENT_ID must share one origin")
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid config: %s", strings.Join(problems, "; "))
	}
	return nil
}

func (c Config) backlineHandoffProblems() []string {
	rawURL, secret := strings.TrimSpace(c.BacklineHandoffURL), c.BacklineHandoffSecret
	if rawURL == "" && secret == "" {
		return nil
	}
	var problems []string
	if rawURL == "" || secret == "" {
		problems = append(problems, "BACKLINE_HANDOFF_URL and BACKLINE_HANDOFF_SECRET must be set together")
	}
	if secret != "" && (len(secret) < 32 || strings.IndexFunc(secret, unicode.IsSpace) >= 0) {
		problems = append(problems, "BACKLINE_HANDOFF_SECRET must be at least 32 characters without whitespace")
	}
	if rawURL != "" {
		parsed, err := url.Parse(rawURL)
		production := strings.EqualFold(strings.TrimSpace(c.AppEnv), "production")
		if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" ||
			(parsed.Scheme != "https" && (production || parsed.Scheme != "http")) {
			problems = append(problems, "BACKLINE_HANDOFF_URL must be an absolute HTTPS URL (HTTP only outside production)")
		}
	}
	return problems
}

func (c Config) atprotoOAuthSettings() atprotocol.OAuthClientSettings {
	return atprotocol.OAuthClientSettings{
		ClientID: c.ATProtoOAuthClientID, CallbackURL: c.ATProtoOAuthCallback,
		JWKSURL: c.ATProtoOAuthJWKSURL, PrivateKey: c.ATProtoOAuthKey, KeyID: c.ATProtoOAuthKeyID,
		PreviousPrivateKey: c.ATProtoOAuthKeyPrevious, PreviousKeyID: c.ATProtoOAuthKeyIDPrevious,
		UserAgent: "subcult-os/0.1", ClientName: "Subcult OS", ClientHomepageURL: c.PublicWebURL,
		PolicyURL: strings.TrimRight(c.PublicWebURL, "/") + "/privacy",
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func parseEnvBool(value string) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	return parsed
}

func (c Config) mailProvider() string {
	if c.MailProvider == "brevo" {
		return "brevo"
	}
	return "resend"
}
