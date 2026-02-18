# AmityVox Phase 4 Pre-Release Security Audit

**Audit Date:** 2026-02-18
**Auditor:** Automated static analysis (Claude Opus 4.6)
**Scope:** Go backend (`internal/`), SvelteKit frontend (`web/`), configuration, deployment
**Codebase Version:** v1.0+ (commit 804f1d3, branch `main`)

---

## Executive Summary

The AmityVox codebase demonstrates strong foundational security: parameterized SQL queries throughout (no SQL injection vectors found), Argon2id password hashing, 256-bit session token entropy, fail-closed WebSocket dispatch, and well-structured permission bitfields. However, several issues were identified across authentication, authorization, input validation, and data exposure categories.

**Totals:** 3 Critical, 5 High, 6 Medium, 5 Low

---

## Critical Severity

### CRIT-01: Session Token Exposure via Sessions List API

- **Description:** `HandleGetSelfSessions` returns the session `id` field for every active session. In AmityVox, the session ID **is** the Bearer authentication token (it is the primary key of `user_sessions` and is used directly in the `Authorization: Bearer <token>` header). This means the `/api/v1/users/@me/sessions` endpoint returns all of a user's valid authentication tokens in plaintext. An attacker who gains access to one session (e.g., via XSS or a stolen device) can harvest tokens for all other sessions, escalating from one compromised session to total account takeover across all devices.
- **File:** `/docker/AmityVox/internal/api/users/users.go:852-876`
- **Severity:** Critical
- **Exploitability:** High. Requires one valid session token (obtainable via XSS, network sniffing, or physical access to one device). The API call is a simple authenticated GET request.
- **Evidence:**
  ```go
  // Line 840-844: Query selects session id (which IS the auth token)
  `SELECT id, user_id, device_name, user_agent, created_at, last_active_at, expires_at
   FROM user_sessions WHERE user_id = $1`

  // Line 863-864: Returns it directly in the response
  session := map[string]interface{}{
      "id": s.ID,  // This is the Bearer token
  ```
- **Recommended Fix:** Never return the raw session ID. Generate a separate non-secret identifier (e.g., a hash or a dedicated `session_ref` UUID column) for the session listing. Mark the current session with a boolean `"current": true` flag (already done) but use the non-secret identifier for the delete endpoint. Alternatively, return only a truncated prefix (first 8 chars) for display purposes.

---

### CRIT-02: Search Results Bypass Channel/Guild Access Controls

- **Description:** The `handleSearchMessages` handler queries Meilisearch for matching messages and hydrates them from the database, but **never checks whether the requesting user has access to the channels or guilds** those messages belong to. An authenticated user can search for and retrieve the full content of messages from private channels, DM channels, and guilds they are not a member of. The optional `guild_id` and `channel_id` filters are user-supplied convenience filters, not access controls. Even if the user specifies no filters, Meilisearch returns results from all indexed messages.
- **File:** `/docker/AmityVox/internal/api/search_handlers.go:20-133`
- **Severity:** Critical
- **Exploitability:** High. Any authenticated user can call `GET /api/v1/search/messages?q=<term>` and receive messages from channels they cannot access. No special privileges required.
- **Evidence:**
  ```go
  // Line 32-36: Only checks authentication, not authorization
  userID := auth.UserIDFromContext(r.Context())
  if userID == "" {
      WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
      return
  }
  // Lines 40-70: Filters are user-provided, not access-control filters
  // Lines 90-96: Hydrates directly from DB with no access check
  rows, err := s.DB.Pool.Query(r.Context(),
      `SELECT ... FROM messages WHERE id = ANY($1)`, result.IDs)
  ```
- **Recommended Fix:** Before returning results, filter out messages from channels the user cannot access. Two approaches:
  1. **Pre-filter (preferred):** Before querying Meilisearch, build a Meilisearch filter that restricts results to channels the user has ViewChannel permission in. Query the user's guild memberships and accessible channels, then add `channel_id IN [...]` to the Meilisearch filter.
  2. **Post-filter:** After hydrating messages, batch-check channel access for each unique channel_id in the results and remove unauthorized messages. This is simpler but may return fewer results than the requested limit.

---

### CRIT-03: Outgoing Webhook SSRF (Server-Side Request Forgery)

- **Description:** `DeliverOutgoingWebhook` makes HTTP POST requests to arbitrary user-configured URLs with no restrictions on the target. An attacker who can create an outgoing webhook (requires ManageWebhooks permission in a guild) can target internal network addresses (e.g., `http://169.254.169.254/` for cloud metadata, `http://localhost:5432/` for PostgreSQL, `http://nats:4222/` for NATS, or any internal Docker network address). The HTTP client follows redirects by default, so even URL validation could be bypassed via a redirect chain.
- **File:** `/docker/AmityVox/internal/api/webhooks/webhooks.go:620-651`
- **Severity:** Critical
- **Exploitability:** Medium. Requires ManageWebhooks permission in at least one guild. The attacker configures the webhook URL to point at an internal service. Events trigger the delivery automatically.
- **Evidence:**
  ```go
  // Line 623-624: No URL validation, no IP restrictions, default redirect policy
  client := &http.Client{Timeout: 10 * time.Second}
  req, err := http.NewRequestWithContext(ctx, http.MethodPost, outgoingURL, bytes.NewReader(payload))
  // Line 633: Sends the request to arbitrary URL
  resp, err := client.Do(req)
  ```
  Note: The federation module **does** have SSRF protection (`ValidateFederationDomain` at `internal/federation/federation.go:246-268`) that blocks private IPs, loopback, and internal domains. This same protection is not applied to outgoing webhooks.
- **Recommended Fix:**
  1. Resolve the webhook URL's hostname to IP addresses before making the request.
  2. Block private/reserved IP ranges (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 127.0.0.0/8, 169.254.0.0/16, ::1, fc00::/7, fe80::/10).
  3. Set `CheckRedirect` on the http.Client to validate each redirect URL against the same blocklist.
  4. Consider reusing the federation module's `ValidateFederationDomain` logic.
  5. Block `http://` URLs in production (require HTTPS for external webhook endpoints).

---

## High Severity

### HIGH-01: WebSocket Gateway Accepts Any Origin

- **Description:** The WebSocket gateway accepts connections from any origin via `OriginPatterns: []string{"*"}`. This means any website can establish a WebSocket connection to the AmityVox gateway. Combined with the user's authentication token (which the malicious page would need to obtain separately), this enables cross-site WebSocket hijacking. Even without the token, the wildcard origin allows probing and fingerprinting.
- **File:** `/docker/AmityVox/internal/gateway/gateway.go:201`
- **Severity:** High
- **Exploitability:** Medium. Requires the attacker to also obtain the user's session token (e.g., via XSS or social engineering). The WebSocket alone cannot authenticate, but the open origin check removes one layer of defense.
- **Evidence:**
  ```go
  OriginPatterns: []string{"*"},
  ```
- **Recommended Fix:** Configure `OriginPatterns` from the server's allowed origins config (same as CORS origins). In development, allow `localhost` origins. In production, restrict to the instance's domain(s).

---

### HIGH-02: TOTP Validation Uses Non-Constant-Time Comparison

- **Description:** The `validateTOTP` function compares user-supplied TOTP codes using Go's `==` operator, which is not constant-time. This theoretically allows a timing side-channel attack where an attacker can determine how many leading characters of the TOTP code are correct by measuring response times. The same non-constant-time comparison appears in `mfa_handlers.go:390-399`.
- **File:** `/docker/AmityVox/internal/auth/auth.go:526`
- **Severity:** High
- **Exploitability:** Low-Medium. TOTP codes are 6 digits with a 30-second validity window (with +/-1 step drift = 90 seconds effective). The timing difference for string comparison of 6-character strings is extremely small (nanoseconds), making practical exploitation very difficult over a network. However, it violates cryptographic best practices.
- **Evidence:**
  ```go
  // Line 526: Uses == instead of subtle.ConstantTimeCompare
  if generateTOTPCode(secret, timeStep+offset) == code {
      return true
  }
  ```
  Note: The webhook token comparison at `webhooks.go:755` correctly uses `subtle.ConstantTimeCompare`, demonstrating that the pattern is known in the codebase.
- **Recommended Fix:** Replace `==` with `subtle.ConstantTimeCompare([]byte(generated), []byte(code)) == 1` in both `auth.go:526` and `mfa_handlers.go`. This is a one-line change.

---

### HIGH-03: Registration Token Timing Side-Channel

- **Description:** When registration requires a token, the server queries the database for the token using `SELECT ... WHERE id = $1`. If the token does not exist, the query returns `sql.ErrNoRows` and the server responds with "Invalid registration token". If the token exists but is exhausted or expired, different error messages are returned. The database query time differs measurably between existing and non-existing tokens (index lookup vs. no rows), potentially allowing an attacker to enumerate valid registration tokens.
- **File:** `/docker/AmityVox/internal/api/server.go:1155-1168`
- **Severity:** High
- **Exploitability:** Medium. Registration tokens are typically ULIDs (26 characters, 128-bit entropy), making brute-force infeasible. However, if tokens are shorter or predictable, timing differences could be exploited. The different error messages ("invalid_token" vs. "token_exhausted" vs. "token_expired") also leak token state information.
- **Evidence:**
  ```go
  // Line 1155-1156: Database lookup leaks existence via timing
  err := s.DB.Pool.QueryRow(r.Context(),
      `SELECT uses, max_uses, expires_at FROM registration_tokens WHERE id = $1`, token).Scan(...)
  // Line 1157-1158: Different error for non-existent
  if err != nil {
      WriteError(w, http.StatusForbidden, "invalid_token", "Invalid registration token")
  // Line 1161-1163: Different error for exhausted
  if uses >= maxUses {
      WriteError(w, http.StatusForbidden, "token_exhausted", "...")
  // Line 1165-1168: Different error for expired
  if expiresAt != nil && expiresAt.Before(time.Now()) {
      WriteError(w, http.StatusForbidden, "token_expired", "...")
  ```
- **Recommended Fix:**
  1. Return a single generic error message for all token validation failures: "Invalid or expired registration token".
  2. Add a constant-time delay (e.g., 200-500ms with small random jitter) to the registration endpoint to mask database timing.
  3. Rate limit registration attempts by IP (already partially done via global rate limiting, but a tighter per-IP limit for registration specifically would help).

---

### HIGH-04: X-Forwarded-For Header Trusted Without Validation

- **Description:** Multiple locations trust the `X-Forwarded-For` header directly without validation. This header can be trivially spoofed by clients. It is used for: (a) rate limiting IP identification (`clientIP` in `ratelimit.go:233-236`), allowing rate limit bypass; (b) session IP tracking in registration (`server.go:1172`) and login (`server.go:1208`), poisoning audit logs; (c) security middleware rate limiting (`middleware/security.go`).
- **Files:**
  - `/docker/AmityVox/internal/api/ratelimit.go:233-236`
  - `/docker/AmityVox/internal/api/server.go:1172-1174`
  - `/docker/AmityVox/internal/api/server.go:1208-1210`
- **Severity:** High
- **Exploitability:** High. Any client can set `X-Forwarded-For` to an arbitrary value. Rate limiting based on this header is trivially bypassable by rotating the header value with each request. This also poisons IP-based audit trails.
- **Evidence:**
  ```go
  // ratelimit.go:234-236
  func clientIP(r *http.Request) string {
      if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
          return strings.Split(fwd, ",")[0]
      }

  // server.go:1172-1174
  ip := r.RemoteAddr
  if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
      ip = fwd
  }
  ```
- **Recommended Fix:** Since AmityVox runs behind Caddy (reverse proxy), the deployment architecture is known. Options:
  1. **Use chi's `RealIP` middleware** (already in the middleware stack at `server.go`) which handles `X-Forwarded-For` with some care. However, remove the manual `X-Forwarded-For` parsing in `clientIP()`, `handleRegister()`, and `handleLogin()` and instead use `r.RemoteAddr` (which `RealIP` middleware has already set correctly).
  2. **Configure trusted proxy CIDR ranges** in the config file. Only accept `X-Forwarded-For` from requests originating from the Caddy container's IP range (typically `172.x.x.x` in Docker networks).

---

### HIGH-05: Admin Routes Lack Middleware-Level Authorization

- **Description:** The admin route group (lines 963-1073 in `server.go`) is nested inside the authenticated route group (so `RequireAuth` middleware runs), but there is no admin-specific middleware. Each individual handler calls `h.isAdmin(r)` to check the admin flag. This is fragile: if any new admin handler forgets to call `isAdmin()`, it becomes accessible to all authenticated users. There are 60+ admin routes, each requiring the manual check.
- **File:** `/docker/AmityVox/internal/api/server.go:963-1073`
- **Severity:** High
- **Exploitability:** Low (currently). All existing handlers appear to call `isAdmin()`. The risk is that a future handler omits the check, which is a common source of privilege escalation bugs.
- **Evidence:**
  ```go
  // Line 963: Admin route group has no admin middleware
  r.Route("/admin", func(r chi.Router) {
      // No r.Use(RequireAdmin) here
      r.Get("/instance", adminH.HandleGetInstance)
      r.Patch("/instance", adminH.HandleUpdateInstance)
      // ... 60+ routes, each must manually call isAdmin()
  ```
- **Recommended Fix:** Create a `RequireAdmin` middleware that checks the admin flag and returns 403 if not set. Apply it to the admin route group:
  ```go
  r.Route("/admin", func(r chi.Router) {
      r.Use(RequireAdmin)
      // handlers no longer need individual isAdmin() calls
  })
  ```

---

## Medium Severity

### MED-01: CORS Wildcard Reflects Arbitrary Origins

- **Description:** When the CORS configuration contains `"*"`, the `corsMiddleware` reflects the exact request `Origin` header back in `Access-Control-Allow-Origin` instead of sending the literal `"*"` value. While the code correctly avoids setting `Access-Control-Allow-Credentials: true` in wildcard mode (line 1462-1465), the origin reflection pattern still allows any origin to make cross-origin requests to the API.
- **File:** `/docker/AmityVox/internal/api/server.go:1440-1477`
- **Severity:** Medium
- **Exploitability:** Medium. In default/production config, if CORS origins are set to `["*"]`, any website can make API requests to the AmityVox instance. Since credentials are not included in wildcard mode, the attack surface is limited to unauthenticated endpoints.
- **Recommended Fix:** In production, never configure CORS origins as `["*"]`. The config template should default to the instance's domain. Add a startup warning if wildcard CORS is detected in production mode.

---

### MED-02: No Content-Type Validation for SVG Uploads

- **Description:** The media upload handler (`HandleUpload`) sniffs content types and allows user-provided types only for image/audio/video prefixes. However, SVG files (which are XML-based and can contain JavaScript) are not explicitly blocked. If an SVG is uploaded and later served without proper content type headers, it could execute JavaScript in the user's browser. The download handler does set `X-Content-Type-Options: nosniff` and a restrictive CSP, which mitigates this partially.
- **File:** `/docker/AmityVox/internal/media/media.go`
- **Severity:** Medium
- **Exploitability:** Low-Medium. The `nosniff` header and CSP on the download endpoint provide defense-in-depth. However, if SVGs are served from a domain that shares cookies with the main application, the CSP may not fully prevent all attack vectors.
- **Recommended Fix:** Explicitly block `image/svg+xml` content type at upload time, or sanitize SVG files by stripping script elements and event handlers before storage. Alternatively, serve user-uploaded files from a separate domain (e.g., `cdn.amityvox.chat`) that shares no cookies with the main domain.

---

### MED-03: READY Payload May Leak Email via Race Condition

- **Description:** The WebSocket READY event sends the authenticated user's full user object. The `User` struct has `Email` tagged with `json:"-"`, so email is not serialized. However, the READY payload also includes the user in `SelfUser` form (which intentionally includes email for the authenticated user's own profile). If other events accidentally include a `SelfUser` object for a different user, emails could leak. The current code appears correct, but this is a design fragility.
- **File:** `/docker/AmityVox/internal/gateway/gateway.go` (READY dispatch, approximately line 611)
- **Severity:** Medium
- **Exploitability:** Low. Currently appears safe due to `json:"-"` tags. The risk is in future code changes that might inadvertently serialize a user object with email included.
- **Recommended Fix:** Add an explicit `user.Email = nil` call before serializing any user object in gateway events (belt-and-suspenders approach), similar to what `HandleGetUser` does at `users.go:167`.

---

### MED-04: No Automatic Session Cleanup for Expired Sessions

- **Description:** Sessions have an `expires_at` timestamp and the `ValidateSession` function checks expiry. However, expired sessions remain in the `user_sessions` table indefinitely until explicitly deleted. Over time, this accumulates stale data and increases the surface area if the session table is ever exposed.
- **File:** `/docker/AmityVox/internal/auth/auth.go` (ValidateSession)
- **Severity:** Medium
- **Exploitability:** None directly. This is a hygiene issue that increases blast radius in case of a database breach.
- **Recommended Fix:** Add a periodic background worker (or a PostgreSQL scheduled job) that runs `DELETE FROM user_sessions WHERE expires_at < NOW()` on a regular interval (e.g., hourly). This reduces the number of valid-looking tokens an attacker could harvest from a database dump.

---

### MED-05: In-Memory DM Spam Tracker Not Shared Across Instances

- **Description:** The DM spam prevention system uses an in-memory map (`dmSpamTracker` in `channels.go`) to track rapid DM creation. In a multi-instance deployment (which the architecture supports via stateless design), each instance has its own tracker. An attacker could distribute DM spam across instances to bypass the limit.
- **File:** `/docker/AmityVox/internal/api/channels/channels.go:42-127`
- **Severity:** Medium
- **Exploitability:** Low. Only relevant in multi-instance deployments. Single-instance deployments (the common case for self-hosted) are unaffected.
- **Recommended Fix:** Move the DM spam tracking to DragonflyDB (Redis-compatible) using atomic INCR with TTL. This makes the counter shared across all instances and also survives restarts.

---

### MED-06: Session Validation Does Not Use Constant-Time Comparison

- **Description:** Session token lookup is performed via a database query (`SELECT ... WHERE id = $1`), where the token is the session ID. Database index lookups are not constant-time, but the timing difference is negligible and hidden behind network/database latency. This is noted for completeness but is standard practice and not practically exploitable.
- **File:** `/docker/AmityVox/internal/auth/auth.go` (ValidateSession)
- **Severity:** Medium (theoretical)
- **Exploitability:** Very Low. Session tokens are 64 hex characters (256 bits). Timing attacks against database lookups over a network are not practically feasible with this entropy level.
- **Recommended Fix:** No immediate action required. If defense-in-depth is desired, consider storing a hash of the session token in the database and looking up by hash prefix, then comparing the full hash with `subtle.ConstantTimeCompare`.

---

## Low Severity

### LOW-01: Default Argon2id Parameters

- **Description:** Password hashing uses `argon2id.DefaultParams` from the alexedwards/argon2id library. The defaults (memory: 64MB, iterations: 1, parallelism: 2) are reasonable for general use but may be conservative for a security-focused application. The memory parameter is the most important for resistance against GPU/ASIC attacks.
- **File:** `/docker/AmityVox/internal/auth/auth.go:117`
- **Severity:** Low
- **Exploitability:** Very Low. The defaults provide adequate protection. This is an optimization suggestion, not a vulnerability.
- **Recommended Fix:** Consider increasing memory to 128MB if the target hardware (Raspberry Pi 5, 8GB RAM) can support it. Make Argon2id parameters configurable via TOML config so administrators can tune for their hardware.

---

### LOW-02: Webhook Token Visible in Execution URL

- **Description:** Incoming webhook execution URLs contain the webhook token as a path parameter (`/api/v1/webhooks/{webhookID}/{token}/execute`). These URLs may appear in server access logs, browser history, and referrer headers. The token comparison itself uses `subtle.ConstantTimeCompare` (correct), but URL-based tokens have inherent leakage risks.
- **File:** `/docker/AmityVox/internal/api/webhooks/webhooks.go`
- **Severity:** Low
- **Exploitability:** Low. Requires access to server logs or network traffic to extract the token. This is the standard pattern used by Discord and Slack, so it is industry-accepted.
- **Recommended Fix:** No action required for compatibility. For enhanced security, consider supporting HMAC-signed request bodies as an alternative authentication method for incoming webhooks.

---

### LOW-03: Federation Signature Parsing Uses fmt.Sscanf

- **Description:** The federation signature verification code uses `fmt.Sscanf` for parsing hex-encoded signatures instead of the more robust `encoding/hex.DecodeString`. While not directly exploitable, `fmt.Sscanf` has less strict error handling and could accept malformed input that `hex.DecodeString` would reject.
- **File:** `/docker/AmityVox/internal/federation/federation.go` (VerifySignature)
- **Severity:** Low
- **Exploitability:** Very Low. The signature is subsequently verified against the Ed25519 public key, so malformed input would fail verification regardless.
- **Recommended Fix:** Replace `fmt.Sscanf` with `hex.DecodeString` for cleaner error handling and stricter input parsing.

---

### LOW-04: No Per-Endpoint Brute Force Protection on Login

- **Description:** Login is protected by the global rate limiter (100/min unauthenticated), but there is no per-account lockout or exponential backoff. An attacker can attempt 100 passwords per minute against a single account before being rate-limited.
- **File:** `/docker/AmityVox/internal/api/server.go` (handleLogin, line 1199)
- **Severity:** Low
- **Exploitability:** Low. Argon2id hashing adds server-side latency (~300ms per attempt), and the rate limiter caps attempts at 100/min. Combined with strong password requirements, brute force is impractical. TOTP (if enabled) adds a second factor.
- **Recommended Fix:** Add per-account failed login tracking: after N consecutive failures (e.g., 5), require a CAPTCHA or impose a progressive delay. Store the failure count in DragonflyDB with a TTL.

---

### LOW-05: In-Memory DM Spam Tracker State Lost on Restart

- **Description:** The DM spam tracker is in-memory only. When the server restarts, all tracking state is lost, briefly allowing a burst of DM spam immediately after a restart.
- **File:** `/docker/AmityVox/internal/api/channels/channels.go:42-127`
- **Severity:** Low
- **Exploitability:** Very Low. Requires knowledge of server restart timing. The window is very brief.
- **Recommended Fix:** Same as MED-05: move to DragonflyDB for persistence across restarts.

---

## Positive Security Findings

The following security practices are well-implemented and deserve recognition:

1. **No SQL injection vectors found.** All database queries use parameterized queries (`$1`, `$2`, etc.) via pgx. No string concatenation in SQL was detected across the entire codebase.

2. **Strong session token entropy.** 32 bytes from `crypto/rand`, hex-encoded to 64 characters (256 bits of entropy). Exceeds OWASP recommendations.

3. **Fail-closed WebSocket dispatch.** The `shouldDispatchTo` function in the gateway defaults to `return false`, ensuring that unrecognized events are never delivered to unauthorized users.

4. **Permission system is comprehensive.** The 9-step permission resolution algorithm (guild owner bypass, @everyone base, role stack, Administrator bypass, channel overrides, timeout mask, ViewChannel gate) is well-structured and correctly implements a fail-closed model.

5. **Webhook token uses constant-time comparison.** `subtle.ConstantTimeCompare` at `webhooks.go:755`.

6. **Federation has SSRF protection.** `ValidateFederationDomain` blocks private IPs, loopback, link-local, and internal domains. Discovery validates domains at each redirect hop.

7. **Sensitive fields use json:"-" tags.** `PasswordHash`, `TOTPSecret`, and `Email` on the `User` struct are excluded from JSON serialization by default. Email is only exposed via the explicit `SelfUser` wrapper for the authenticated user's own profile.

8. **File downloads set security headers.** `X-Content-Type-Options: nosniff` and a restrictive CSP are set on file download responses.

9. **Input validation on search filters.** Search query parameters are validated against a ULID regex pattern (`^[A-Za-z0-9]{26}$`) to prevent filter injection into Meilisearch queries.

10. **Password breach checking.** HaveIBeenPwned k-anonymity integration (via `middleware/security.go`) warns users about compromised passwords without sending the full hash to an external service.

---

## Remediation Priority

| Priority | ID | Summary | Effort |
|----------|----|---------|--------|
| 1 | CRIT-01 | Stop returning session tokens in session list | Small (1-2 hours) |
| 2 | CRIT-02 | Add access control to message search | Medium (4-8 hours) |
| 3 | CRIT-03 | Add SSRF protection to outgoing webhooks | Small (2-3 hours) |
| 4 | HIGH-04 | Fix X-Forwarded-For trust chain | Small (1-2 hours) |
| 5 | HIGH-05 | Add RequireAdmin middleware | Small (1-2 hours) |
| 6 | HIGH-02 | Use constant-time TOTP comparison | Trivial (15 minutes) |
| 7 | HIGH-01 | Restrict WebSocket origin patterns | Small (1 hour) |
| 8 | HIGH-03 | Normalize registration token errors | Small (1 hour) |
| 9 | MED-01 | Warn on wildcard CORS in production | Small (30 minutes) |
| 10 | MED-02 | Block SVG uploads or sanitize | Small (1-2 hours) |
| 11 | MED-04 | Add expired session cleanup worker | Small (1-2 hours) |
| 12 | MED-05 | Move DM spam tracker to DragonflyDB | Medium (2-4 hours) |
| 13 | MED-03 | Add defensive email nil in gateway | Trivial (15 minutes) |
| 14 | MED-06 | (No action required) | N/A |
| 15 | LOW-01 | Make Argon2id params configurable | Small (1 hour) |
| 16 | LOW-04 | Add per-account login throttling | Medium (2-4 hours) |
| 17 | LOW-02 | (No action required - industry standard) | N/A |
| 18 | LOW-03 | Replace Sscanf with hex.DecodeString | Trivial (15 minutes) |
| 19 | LOW-05 | (Covered by MED-05) | N/A |

**Estimated total remediation effort:** 18-32 hours for all actionable items.

---

## Methodology

This audit was performed through static analysis of the Go backend source code. The following techniques were used:

1. **Manual code review** of authentication (`auth/`), authorization (`permissions/`), middleware (`middleware/`), and API handler (`api/`) packages.
2. **Pattern-based search** for known vulnerability patterns: SQL concatenation (`fmt.Sprintf` near SQL), `X-Forwarded-For` trust, non-constant-time comparisons, wildcard CORS/origin, SSRF vectors.
3. **Data flow tracing** from user input through handlers to database queries and HTTP responses.
4. **Cross-reference validation** between route registration (`server.go`) and handler implementations to verify authorization coverage.

**Limitations:** This is a static analysis audit. It does not include:
- Dynamic/runtime testing (fuzzing, penetration testing)
- Dependency vulnerability scanning (SCA)
- Infrastructure/deployment configuration review beyond Docker Compose
- Frontend JavaScript/Svelte security analysis (XSS in rendered content, CSP bypass, etc.)
- Performance/DoS testing under load
