package middleware

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
)

// CSPConfig defines Content Security Policy directives for the AmityVox frontend.
// Each field maps to a CSP directive. Empty fields are omitted from the header.
type CSPConfig struct {
	// DefaultSrc is the fallback for other directives. Defaults to "'self'".
	DefaultSrc []string

	// ScriptSrc controls allowed script sources. Defaults to "'self'".
	ScriptSrc []string

	// StyleSrc controls allowed stylesheet sources. Defaults to "'self' 'unsafe-inline'"
	// because Svelte and Tailwind inject inline styles.
	StyleSrc []string

	// ImgSrc controls allowed image sources. Defaults to "'self' data: blob:".
	ImgSrc []string

	// ConnectSrc controls allowed connection targets (fetch, WebSocket, etc.).
	// Defaults to "'self' wss: ws:" to allow WebSocket gateway connections.
	ConnectSrc []string

	// FontSrc controls allowed font sources. Defaults to "'self'".
	FontSrc []string

	// MediaSrc controls allowed media (audio/video) sources.
	MediaSrc []string

	// ObjectSrc controls allowed plugin sources. Defaults to "'none'".
	ObjectSrc []string

	// FrameSrc controls allowed iframe sources. Defaults to "'none'".
	FrameSrc []string

	// FrameAncestors controls which parents can embed this page. Defaults to "'none'".
	FrameAncestors []string

	// BaseURI restricts the base element. Defaults to "'self'".
	BaseURI []string

	// FormAction restricts form submission targets. Defaults to "'self'".
	FormAction []string

	// WorkerSrc controls allowed Worker/SharedWorker/ServiceWorker sources.
	WorkerSrc []string

	// ManifestSrc controls allowed web app manifest sources.
	ManifestSrc []string

	// UpgradeInsecureRequests adds the upgrade-insecure-requests directive.
	UpgradeInsecureRequests bool

	// ReportURI is the endpoint to send CSP violation reports to.
	ReportURI string

	// UseNonce enables per-request nonce generation for script and style sources.
	UseNonce bool
}

// DefaultCSPConfig returns a secure default CSP configuration suitable for
// the AmityVox SvelteKit frontend.
func DefaultCSPConfig() CSPConfig {
	return CSPConfig{
		DefaultSrc:              []string{"'self'"},
		ScriptSrc:               []string{"'self'"},
		StyleSrc:                []string{"'self'", "'unsafe-inline'"},
		ImgSrc:                  []string{"'self'", "data:", "blob:", "https://media.tenor.com", "https://media1.tenor.com", "https://media.giphy.com"},
		ConnectSrc:              []string{"'self'", "wss:", "ws:", "https://api.giphy.com"},
		FontSrc:                 []string{"'self'"},
		MediaSrc:                []string{"'self'", "blob:"},
		ObjectSrc:               []string{"'none'"},
		FrameSrc:                []string{"'none'"},
		FrameAncestors:          []string{"'none'"},
		BaseURI:                 []string{"'self'"},
		FormAction:              []string{"'self'"},
		WorkerSrc:               []string{"'self'"},
		ManifestSrc:             []string{"'self'"},
		UpgradeInsecureRequests: true,
		UseNonce:                false,
	}
}

// contextKey for CSP nonce is defined in tracing.go using the same type.
const cspNonceKey contextKey = "csp_nonce"

// ContentSecurityPolicy returns a middleware that sets the Content-Security-Policy
// header on all responses. When UseNonce is enabled, a unique cryptographic nonce
// is generated per request and added to script-src and style-src directives.
// The nonce is stored in the request context for use by templates.
func ContentSecurityPolicy(cfg CSPConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var nonce string
			if cfg.UseNonce {
				nonceBytes := make([]byte, 16)
				if _, err := rand.Read(nonceBytes); err == nil {
					nonce = base64.StdEncoding.EncodeToString(nonceBytes)
				}
			}

			policy := buildCSPHeader(cfg, nonce)
			w.Header().Set("Content-Security-Policy", policy)

			if nonce != "" {
				r = r.WithContext(withCSPNonce(r.Context(), nonce))
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GetCSPNonce retrieves the CSP nonce from the request context. Returns an empty
// string if no nonce is present (nonce generation disabled).
func GetCSPNonce(r *http.Request) string {
	if nonce, ok := r.Context().Value(cspNonceKey).(string); ok {
		return nonce
	}
	return ""
}

// withCSPNonce stores the CSP nonce in the given context.
func withCSPNonce(ctx context.Context, nonce string) context.Context {
	return context.WithValue(ctx, cspNonceKey, nonce)
}

// buildCSPHeader constructs the CSP header value from the configuration.
func buildCSPHeader(cfg CSPConfig, nonce string) string {
	var directives []string

	addDirective := func(name string, sources []string) {
		if len(sources) > 0 {
			// If nonce is set, add it to script-src and style-src.
			if nonce != "" && (name == "script-src" || name == "style-src") {
				sources = append(sources, fmt.Sprintf("'nonce-%s'", nonce))
			}
			directives = append(directives, fmt.Sprintf("%s %s", name, strings.Join(sources, " ")))
		}
	}

	addDirective("default-src", cfg.DefaultSrc)
	addDirective("script-src", cfg.ScriptSrc)
	addDirective("style-src", cfg.StyleSrc)
	addDirective("img-src", cfg.ImgSrc)
	addDirective("connect-src", cfg.ConnectSrc)
	addDirective("font-src", cfg.FontSrc)
	addDirective("media-src", cfg.MediaSrc)
	addDirective("object-src", cfg.ObjectSrc)
	addDirective("frame-src", cfg.FrameSrc)
	addDirective("frame-ancestors", cfg.FrameAncestors)
	addDirective("base-uri", cfg.BaseURI)
	addDirective("form-action", cfg.FormAction)
	addDirective("worker-src", cfg.WorkerSrc)
	addDirective("manifest-src", cfg.ManifestSrc)

	if cfg.UpgradeInsecureRequests {
		directives = append(directives, "upgrade-insecure-requests")
	}

	if cfg.ReportURI != "" {
		directives = append(directives, fmt.Sprintf("report-uri %s", cfg.ReportURI))
	}

	return strings.Join(directives, "; ")
}

// SecurityHeaders returns a middleware that sets common security headers on all
// responses, complementing the CSP middleware.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent MIME type sniffing.
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking.
		w.Header().Set("X-Frame-Options", "DENY")

		// Enable XSS protection (legacy browsers).
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Control referrer information leakage.
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Restrict browser features via Permissions-Policy.
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(self), geolocation=(), payment=()")

		// Strict Transport Security (HSTS) — 1 year with subdomains.
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")

		// Prevent cross-origin resource leakage.
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")

		next.ServeHTTP(w, r)
	})
}
