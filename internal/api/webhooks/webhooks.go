// Package webhooks implements the webhook execution endpoint and webhook
// management features including templates, execution logs, preview, and
// outgoing webhook delivery. Incoming webhooks allow external services to post
// messages to channels using a webhook ID and token pair, without requiring
// Bearer auth. Mounted at /api/v1/webhooks/{webhookID}/{token}.
package webhooks

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
)

// Handler implements webhook-related REST API endpoints.
type Handler struct {
	Pool     *pgxpool.Pool
	EventBus *events.Bus
	Logger   *slog.Logger
}

type executeWebhookRequest struct {
	Content    string  `json:"content"`
	Username   *string `json:"username"`
	AvatarURL  *string `json:"avatar_url"`
	TemplateID *string `json:"template_id"`
}

// --- Webhook Templates (in-memory) ---

// WebhookTemplate defines a built-in payload transformation template for
// common external services (GitHub, GitLab, Jira, Sentry).
type WebhookTemplate struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	SamplePayload string `json:"sample_payload"`
	Service       string `json:"service"`
}

// templateTransformResult holds the output of a template transformation.
type templateTransformResult struct {
	Content string                   `json:"content"`
	Embeds  []map[string]interface{} `json:"embeds,omitempty"`
}

// builtinTemplates contains all available webhook templates.
var builtinTemplates = []WebhookTemplate{
	{
		ID:          "github-push",
		Name:        "GitHub Push",
		Description: "Formats GitHub push event payloads into channel messages showing commits.",
		Service:     "github",
		SamplePayload: `{"ref":"refs/heads/main","pusher":{"name":"octocat"},"repository":{"full_name":"octocat/Hello-World"},"commits":[{"id":"abc123def456","message":"Fix bug in login flow","author":{"name":"Octocat"}}],"compare":"https://github.com/octocat/Hello-World/compare/abc...def"}`,
	},
	{
		ID:          "github-pr",
		Name:        "GitHub Pull Request",
		Description: "Formats GitHub pull request event payloads showing PR details.",
		Service:     "github",
		SamplePayload: `{"action":"opened","number":42,"pull_request":{"title":"Add new feature","html_url":"https://github.com/octocat/Hello-World/pull/42","user":{"login":"octocat"},"body":"This PR adds a new feature.","head":{"ref":"feature-branch"},"base":{"ref":"main"},"merged":false,"draft":false}},"repository":{"full_name":"octocat/Hello-World"}}`,
	},
	{
		ID:          "github-issues",
		Name:        "GitHub Issues",
		Description: "Formats GitHub issue event payloads showing issue details.",
		Service:     "github",
		SamplePayload: `{"action":"opened","issue":{"number":1,"title":"Found a bug","html_url":"https://github.com/octocat/Hello-World/issues/1","user":{"login":"octocat"},"body":"Something is broken.","labels":[{"name":"bug","color":"d73a4a"}]},"repository":{"full_name":"octocat/Hello-World"}}`,
	},
	{
		ID:          "gitlab-push",
		Name:        "GitLab Push",
		Description: "Formats GitLab push event payloads into channel messages.",
		Service:     "gitlab",
		SamplePayload: `{"ref":"refs/heads/main","user_name":"root","project":{"path_with_namespace":"root/my-project","web_url":"https://gitlab.example.com/root/my-project"},"commits":[{"id":"abc123","message":"Update README","author":{"name":"Root"}}],"total_commits_count":1}`,
	},
	{
		ID:          "gitlab-mr",
		Name:        "GitLab Merge Request",
		Description: "Formats GitLab merge request event payloads.",
		Service:     "gitlab",
		SamplePayload: `{"object_kind":"merge_request","user":{"username":"root"},"project":{"path_with_namespace":"root/my-project","web_url":"https://gitlab.example.com/root/my-project"},"object_attributes":{"title":"Add new feature","url":"https://gitlab.example.com/root/my-project/-/merge_requests/1","action":"open","source_branch":"feature","target_branch":"main","description":"New feature description","iid":1,"state":"opened"}}`,
	},
	{
		ID:          "jira-issue",
		Name:        "Jira Issue",
		Description: "Formats Jira issue event payloads showing issue updates.",
		Service:     "jira",
		SamplePayload: `{"webhookEvent":"jira:issue_created","user":{"displayName":"John Doe"},"issue":{"key":"PROJ-123","fields":{"summary":"Login page broken","issuetype":{"name":"Bug"},"priority":{"name":"High"},"status":{"name":"Open"},"description":"The login page returns 500 error.","assignee":{"displayName":"Jane Smith"}},"self":"https://jira.example.com/rest/api/2/issue/PROJ-123"}}`,
	},
	{
		ID:          "sentry-error",
		Name:        "Sentry Error",
		Description: "Formats Sentry error/issue alert payloads.",
		Service:     "sentry",
		SamplePayload: `{"action":"created","data":{"issue":{"title":"TypeError: Cannot read property 'map' of undefined","culprit":"app/components/UserList.tsx","shortId":"FRONTEND-1K","metadata":{"type":"TypeError","value":"Cannot read property 'map' of undefined"},"count":42,"userCount":12,"firstSeen":"2025-01-15T10:30:00Z","project":{"name":"frontend","slug":"frontend"}},"event":{"event_id":"abc123","platform":"javascript","tags":[{"key":"browser","value":"Chrome 120"}]}},"actor":{"name":"Sentry"}}`,
	},
}

// templatesByID provides fast lookup of templates by ID.
var templatesByID map[string]WebhookTemplate

func init() {
	templatesByID = make(map[string]WebhookTemplate, len(builtinTemplates))
	for _, t := range builtinTemplates {
		templatesByID[t.ID] = t
	}
}

// transformPayload applies the named template to the given raw payload and
// returns the formatted content and optional embeds.
func transformPayload(templateID string, payload json.RawMessage) (*templateTransformResult, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("invalid JSON payload: %w", err)
	}

	switch templateID {
	case "github-push":
		return transformGitHubPush(data)
	case "github-pr":
		return transformGitHubPR(data)
	case "github-issues":
		return transformGitHubIssues(data)
	case "gitlab-push":
		return transformGitLabPush(data)
	case "gitlab-mr":
		return transformGitLabMR(data)
	case "jira-issue":
		return transformJiraIssue(data)
	case "sentry-error":
		return transformSentryError(data)
	default:
		return nil, fmt.Errorf("unknown template: %s", templateID)
	}
}

func getString(m map[string]interface{}, keys ...string) string {
	current := m
	for i, key := range keys {
		val, ok := current[key]
		if !ok {
			return ""
		}
		if i == len(keys)-1 {
			s, _ := val.(string)
			return s
		}
		next, ok := val.(map[string]interface{})
		if !ok {
			return ""
		}
		current = next
	}
	return ""
}

func getFloat(m map[string]interface{}, keys ...string) float64 {
	current := m
	for i, key := range keys {
		val, ok := current[key]
		if !ok {
			return 0
		}
		if i == len(keys)-1 {
			f, _ := val.(float64)
			return f
		}
		next, ok := val.(map[string]interface{})
		if !ok {
			return 0
		}
		current = next
	}
	return 0
}

func getObject(m map[string]interface{}, keys ...string) map[string]interface{} {
	current := m
	for _, key := range keys {
		val, ok := current[key]
		if !ok {
			return nil
		}
		next, ok := val.(map[string]interface{})
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

func getArray(m map[string]interface{}, key string) []interface{} {
	val, ok := m[key]
	if !ok {
		return nil
	}
	arr, ok := val.([]interface{})
	if !ok {
		return nil
	}
	return arr
}

func transformGitHubPush(data map[string]interface{}) (*templateTransformResult, error) {
	ref := getString(data, "ref")
	branch := ref
	if strings.HasPrefix(ref, "refs/heads/") {
		branch = strings.TrimPrefix(ref, "refs/heads/")
	}
	pusher := getString(data, "pusher", "name")
	repo := getString(data, "repository", "full_name")
	compareURL := getString(data, "compare")

	commits := getArray(data, "commits")
	var commitLines []string
	for i, c := range commits {
		if i >= 5 {
			commitLines = append(commitLines, fmt.Sprintf("... and %d more commits", len(commits)-5))
			break
		}
		cm, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		sha := getString(cm, "id")
		if len(sha) > 7 {
			sha = sha[:7]
		}
		msg := getString(cm, "message")
		if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
			msg = msg[:idx]
		}
		if len(msg) > 72 {
			msg = msg[:69] + "..."
		}
		commitLines = append(commitLines, fmt.Sprintf("`%s` %s", sha, msg))
	}

	content := fmt.Sprintf("**[%s]** %s pushed %d commit(s) to `%s`", repo, pusher, len(commits), branch)
	if len(commitLines) > 0 {
		content += "\n" + strings.Join(commitLines, "\n")
	}
	if compareURL != "" {
		content += fmt.Sprintf("\n[View changes](%s)", compareURL)
	}

	return &templateTransformResult{Content: content}, nil
}

func transformGitHubPR(data map[string]interface{}) (*templateTransformResult, error) {
	action := getString(data, "action")
	repo := getString(data, "repository", "full_name")
	pr := getObject(data, "pull_request")
	if pr == nil {
		return &templateTransformResult{Content: fmt.Sprintf("**[%s]** Pull request event: %s", repo, action)}, nil
	}

	title := getString(pr, "title")
	url := getString(pr, "html_url")
	user := getString(pr, "user", "login")
	number := getFloat(data, "number")
	headBranch := getString(pr, "head", "ref")
	baseBranch := getString(pr, "base", "ref")
	body := getString(pr, "body")
	if len(body) > 200 {
		body = body[:197] + "..."
	}

	content := fmt.Sprintf("**[%s]** Pull request #%.0f %s by %s", repo, number, action, user)
	content += fmt.Sprintf("\n**%s** (`%s` -> `%s`)", title, headBranch, baseBranch)
	if body != "" {
		content += fmt.Sprintf("\n> %s", strings.ReplaceAll(body, "\n", "\n> "))
	}
	if url != "" {
		content += fmt.Sprintf("\n[View PR](%s)", url)
	}

	return &templateTransformResult{Content: content}, nil
}

func transformGitHubIssues(data map[string]interface{}) (*templateTransformResult, error) {
	action := getString(data, "action")
	repo := getString(data, "repository", "full_name")
	issue := getObject(data, "issue")
	if issue == nil {
		return &templateTransformResult{Content: fmt.Sprintf("**[%s]** Issue event: %s", repo, action)}, nil
	}

	title := getString(issue, "title")
	url := getString(issue, "html_url")
	user := getString(issue, "user", "login")
	number := getFloat(issue, "number")
	body := getString(issue, "body")
	if len(body) > 200 {
		body = body[:197] + "..."
	}

	// Collect labels.
	labels := getArray(issue, "labels")
	var labelNames []string
	for _, l := range labels {
		lm, ok := l.(map[string]interface{})
		if !ok {
			continue
		}
		labelNames = append(labelNames, getString(lm, "name"))
	}

	content := fmt.Sprintf("**[%s]** Issue #%.0f %s by %s", repo, number, action, user)
	content += fmt.Sprintf("\n**%s**", title)
	if len(labelNames) > 0 {
		content += fmt.Sprintf(" [%s]", strings.Join(labelNames, ", "))
	}
	if body != "" {
		content += fmt.Sprintf("\n> %s", strings.ReplaceAll(body, "\n", "\n> "))
	}
	if url != "" {
		content += fmt.Sprintf("\n[View issue](%s)", url)
	}

	return &templateTransformResult{Content: content}, nil
}

func transformGitLabPush(data map[string]interface{}) (*templateTransformResult, error) {
	ref := getString(data, "ref")
	branch := ref
	if strings.HasPrefix(ref, "refs/heads/") {
		branch = strings.TrimPrefix(ref, "refs/heads/")
	}
	userName := getString(data, "user_name")
	project := getString(data, "project", "path_with_namespace")
	webURL := getString(data, "project", "web_url")

	commits := getArray(data, "commits")
	var commitLines []string
	for i, c := range commits {
		if i >= 5 {
			commitLines = append(commitLines, fmt.Sprintf("... and %d more commits", len(commits)-5))
			break
		}
		cm, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		sha := getString(cm, "id")
		if len(sha) > 7 {
			sha = sha[:7]
		}
		msg := getString(cm, "message")
		if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
			msg = msg[:idx]
		}
		if len(msg) > 72 {
			msg = msg[:69] + "..."
		}
		commitLines = append(commitLines, fmt.Sprintf("`%s` %s", sha, msg))
	}

	content := fmt.Sprintf("**[%s]** %s pushed %d commit(s) to `%s`", project, userName, len(commits), branch)
	if len(commitLines) > 0 {
		content += "\n" + strings.Join(commitLines, "\n")
	}
	if webURL != "" {
		content += fmt.Sprintf("\n[View project](%s)", webURL)
	}

	return &templateTransformResult{Content: content}, nil
}

func transformGitLabMR(data map[string]interface{}) (*templateTransformResult, error) {
	user := getString(data, "user", "username")
	project := getString(data, "project", "path_with_namespace")
	attrs := getObject(data, "object_attributes")
	if attrs == nil {
		return &templateTransformResult{Content: fmt.Sprintf("**[%s]** Merge request event by %s", project, user)}, nil
	}

	title := getString(attrs, "title")
	url := getString(attrs, "url")
	action := getString(attrs, "action")
	source := getString(attrs, "source_branch")
	target := getString(attrs, "target_branch")
	iid := getFloat(attrs, "iid")
	desc := getString(attrs, "description")
	if len(desc) > 200 {
		desc = desc[:197] + "..."
	}

	content := fmt.Sprintf("**[%s]** Merge request !%.0f %s by %s", project, iid, action, user)
	content += fmt.Sprintf("\n**%s** (`%s` -> `%s`)", title, source, target)
	if desc != "" {
		content += fmt.Sprintf("\n> %s", strings.ReplaceAll(desc, "\n", "\n> "))
	}
	if url != "" {
		content += fmt.Sprintf("\n[View MR](%s)", url)
	}

	return &templateTransformResult{Content: content}, nil
}

func transformJiraIssue(data map[string]interface{}) (*templateTransformResult, error) {
	webhookEvent := getString(data, "webhookEvent")
	user := getString(data, "user", "displayName")
	issue := getObject(data, "issue")
	if issue == nil {
		return &templateTransformResult{Content: fmt.Sprintf("Jira event: %s by %s", webhookEvent, user)}, nil
	}

	key := getString(issue, "key")
	fields := getObject(issue, "fields")
	summary := ""
	issueType := ""
	priority := ""
	status := ""
	description := ""
	assignee := ""
	if fields != nil {
		summary = getString(fields, "summary")
		issueType = getString(fields, "issuetype", "name")
		priority = getString(fields, "priority", "name")
		status = getString(fields, "status", "name")
		description = getString(fields, "description")
		assignee = getString(fields, "assignee", "displayName")
	}
	if len(description) > 200 {
		description = description[:197] + "..."
	}

	// Map webhookEvent to a human-readable action.
	action := webhookEvent
	switch {
	case strings.Contains(webhookEvent, "created"):
		action = "created"
	case strings.Contains(webhookEvent, "updated"):
		action = "updated"
	case strings.Contains(webhookEvent, "deleted"):
		action = "deleted"
	}

	content := fmt.Sprintf("**[%s]** %s %s by %s", key, issueType, action, user)
	content += fmt.Sprintf("\n**%s**", summary)

	var meta []string
	if priority != "" {
		meta = append(meta, "Priority: "+priority)
	}
	if status != "" {
		meta = append(meta, "Status: "+status)
	}
	if assignee != "" {
		meta = append(meta, "Assignee: "+assignee)
	}
	if len(meta) > 0 {
		content += "\n" + strings.Join(meta, " | ")
	}
	if description != "" {
		content += fmt.Sprintf("\n> %s", strings.ReplaceAll(description, "\n", "\n> "))
	}

	return &templateTransformResult{Content: content}, nil
}

func transformSentryError(data map[string]interface{}) (*templateTransformResult, error) {
	action := getString(data, "action")
	issueData := getObject(data, "data", "issue")
	if issueData == nil {
		return &templateTransformResult{Content: fmt.Sprintf("Sentry alert: %s", action)}, nil
	}

	title := getString(issueData, "title")
	culprit := getString(issueData, "culprit")
	shortID := getString(issueData, "shortId")
	project := getString(issueData, "project", "name")
	count := getFloat(issueData, "count")
	userCount := getFloat(issueData, "userCount")

	metadata := getObject(issueData, "metadata")
	errType := ""
	errValue := ""
	if metadata != nil {
		errType = getString(metadata, "type")
		errValue = getString(metadata, "value")
	}

	content := fmt.Sprintf("**[%s]** %s %s", project, shortID, action)
	content += fmt.Sprintf("\n**%s**", title)
	if errType != "" && errValue != "" {
		content += fmt.Sprintf("\n`%s: %s`", errType, errValue)
	}
	if culprit != "" {
		content += fmt.Sprintf("\nCulprit: `%s`", culprit)
	}
	if count > 0 || userCount > 0 {
		content += fmt.Sprintf("\nEvents: %.0f | Users: %.0f", count, userCount)
	}

	return &templateTransformResult{Content: content}, nil
}

// --- Execution Log Helpers ---

// WebhookExecutionLog represents a single execution log entry.
type WebhookExecutionLog struct {
	ID              string    `json:"id"`
	WebhookID       string    `json:"webhook_id"`
	StatusCode      int       `json:"status_code"`
	RequestBody     *string   `json:"request_body,omitempty"`
	ResponsePreview *string   `json:"response_preview,omitempty"`
	Success         bool      `json:"success"`
	ErrorMessage    *string   `json:"error_message,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// logExecution inserts a webhook execution log entry and prunes old entries
// to keep at most 100 per webhook.
func (h *Handler) logExecution(ctx context.Context, webhookID string, statusCode int, requestBody, responsePreview string, success bool, errMsg string) {
	logID := models.NewULID().String()

	// Truncate request body to 4000 chars.
	if len(requestBody) > 4000 {
		requestBody = requestBody[:4000]
	}
	// Truncate response preview to 2000 chars.
	if len(responsePreview) > 2000 {
		responsePreview = responsePreview[:2000]
	}

	var reqBodyPtr, respPreviewPtr, errMsgPtr *string
	if requestBody != "" {
		reqBodyPtr = &requestBody
	}
	if responsePreview != "" {
		respPreviewPtr = &responsePreview
	}
	if errMsg != "" {
		errMsgPtr = &errMsg
	}

	_, err := h.Pool.Exec(ctx,
		`INSERT INTO webhook_execution_logs (id, webhook_id, status_code, request_body, response_preview, success, error_message, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, now())`,
		logID, webhookID, statusCode, reqBodyPtr, respPreviewPtr, success, errMsgPtr,
	)
	if err != nil {
		h.Logger.Error("failed to insert webhook execution log",
			slog.String("error", err.Error()),
			slog.String("webhook_id", webhookID),
		)
		return
	}

	// Prune old entries beyond 100 per webhook.
	_, err = h.Pool.Exec(ctx,
		`DELETE FROM webhook_execution_logs
		 WHERE webhook_id = $1 AND id NOT IN (
			SELECT id FROM webhook_execution_logs
			WHERE webhook_id = $1
			ORDER BY created_at DESC
			LIMIT 100
		 )`,
		webhookID,
	)
	if err != nil {
		h.Logger.Error("failed to prune webhook execution logs",
			slog.String("error", err.Error()),
			slog.String("webhook_id", webhookID),
		)
	}
}

// --- Outgoing Webhook Delivery ---

// outgoingEvents lists the valid event types that can trigger outgoing webhooks.
var outgoingEvents = map[string]string{
	"message_create":     events.SubjectMessageCreate,
	"message_update":     events.SubjectMessageUpdate,
	"message_delete":     events.SubjectMessageDelete,
	"member_join":        events.SubjectGuildMemberAdd,
	"member_leave":       events.SubjectGuildMemberRemove,
	"member_ban":         events.SubjectGuildBanAdd,
	"member_unban":       events.SubjectGuildBanRemove,
	"channel_create":     events.SubjectChannelCreate,
	"channel_update":     events.SubjectChannelUpdate,
	"channel_delete":     events.SubjectChannelDelete,
	"guild_update":       events.SubjectGuildUpdate,
	"role_create":        events.SubjectGuildRoleCreate,
	"role_update":        events.SubjectGuildRoleUpdate,
	"role_delete":        events.SubjectGuildRoleDelete,
	"reaction_add":       events.SubjectMessageReactionAdd,
	"reaction_remove":    events.SubjectMessageReactionDel,
}

// ValidOutgoingEvents returns a sorted list of valid outgoing event names.
func ValidOutgoingEvents() []string {
	result := make([]string, 0, len(outgoingEvents))
	for k := range outgoingEvents {
		result = append(result, k)
	}
	return result
}

// DeliverOutgoingWebhook sends the event payload to the outgoing webhook URL
// via HTTP POST. This should be called asynchronously from the event bus
// subscriber. It logs the execution result.
func (h *Handler) DeliverOutgoingWebhook(ctx context.Context, webhookID, outgoingURL string, eventType string, payload json.RawMessage) {
	reqBody := string(payload)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, outgoingURL, bytes.NewReader(payload))
	if err != nil {
		h.logExecution(ctx, webhookID, 0, reqBody, "", false, fmt.Sprintf("failed to create request: %v", err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "AmityVox-Webhook/1.0")
	req.Header.Set("X-AmityVox-Event", eventType)

	resp, err := client.Do(req)
	if err != nil {
		h.logExecution(ctx, webhookID, 0, reqBody, "", false, fmt.Sprintf("request failed: %v", err))
		return
	}
	defer resp.Body.Close()

	// Read up to 2KB of the response for the preview.
	respBodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	respPreview := string(respBodyBytes)

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	errMsg := ""
	if !success {
		errMsg = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	h.logExecution(ctx, webhookID, resp.StatusCode, reqBody, respPreview, success, errMsg)
}

// StartOutgoingWebhookSubscriber subscribes to all NATS event subjects and
// dispatches matching events to outgoing webhooks. Call this once during
// server startup. It runs asynchronously.
func (h *Handler) StartOutgoingWebhookSubscriber() {
	if h.EventBus == nil {
		return
	}

	_, err := h.EventBus.SubscribeWildcard("amityvox.>", func(subject string, event events.Event) {
		// Map subject back to our event type name.
		var eventName string
		for name, subj := range outgoingEvents {
			if subj == subject {
				eventName = name
				break
			}
		}
		if eventName == "" {
			return // not an event we handle for outgoing webhooks
		}

		// Query outgoing webhooks that listen for this event type.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		rows, err := h.Pool.Query(ctx,
			`SELECT id, outgoing_url FROM webhooks
			 WHERE webhook_type = 'outgoing'
			   AND outgoing_url IS NOT NULL
			   AND outgoing_url != ''
			   AND $1 = ANY(outgoing_events)`,
			eventName,
		)
		if err != nil {
			h.Logger.Error("failed to query outgoing webhooks",
				slog.String("error", err.Error()),
				slog.String("event", eventName),
			)
			return
		}
		defer rows.Close()

		type target struct {
			id  string
			url string
		}
		var targets []target
		for rows.Next() {
			var t target
			if err := rows.Scan(&t.id, &t.url); err != nil {
				continue
			}
			targets = append(targets, t)
		}

		// Build the outgoing payload.
		outPayload, _ := json.Marshal(map[string]interface{}{
			"event": eventName,
			"data":  json.RawMessage(event.Data),
		})

		for _, t := range targets {
			go h.DeliverOutgoingWebhook(ctx, t.id, t.url, eventName, outPayload)
		}
	})
	if err != nil {
		h.Logger.Error("failed to start outgoing webhook subscriber",
			slog.String("error", err.Error()),
		)
	} else {
		h.Logger.Info("outgoing webhook subscriber started")
	}
}

// --- HTTP Handlers ---

// HandleExecute handles POST /api/v1/webhooks/{webhookID}/{token}.
// This endpoint does NOT require Bearer auth — the token in the URL is the secret.
func (h *Handler) HandleExecute(w http.ResponseWriter, r *http.Request) {
	webhookID := chi.URLParam(r, "webhookID")
	token := chi.URLParam(r, "token")

	// Look up the webhook and verify the token.
	var wh models.Webhook
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, guild_id, channel_id, creator_id, name, avatar_id, token,
		        webhook_type, outgoing_url, created_at
		 FROM webhooks WHERE id = $1`, webhookID).Scan(
		&wh.ID, &wh.GuildID, &wh.ChannelID, &wh.CreatorID, &wh.Name,
		&wh.AvatarID, &wh.Token, &wh.WebhookType, &wh.OutgoingURL, &wh.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "webhook_not_found", "Unknown webhook")
		return
	}
	if err != nil {
		h.Logger.Error("failed to look up webhook", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to look up webhook")
		return
	}

	// Constant-time token comparison to prevent timing attacks.
	if subtle.ConstantTimeCompare([]byte(wh.Token), []byte(token)) != 1 {
		writeError(w, http.StatusUnauthorized, "invalid_token", "Invalid webhook token")
		return
	}

	if wh.WebhookType != models.WebhookTypeIncoming {
		writeError(w, http.StatusBadRequest, "wrong_type", "This webhook does not accept incoming messages")
		return
	}

	// Read raw body for logging, then decode.
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Failed to read request body")
		return
	}

	var req executeWebhookRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		h.logExecution(r.Context(), webhookID, http.StatusBadRequest, string(bodyBytes), "", false, "Invalid request body")
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// If a template_id is provided, try to transform the raw payload.
	finalContent := req.Content
	if req.TemplateID != nil && *req.TemplateID != "" {
		result, err := transformPayload(*req.TemplateID, bodyBytes)
		if err != nil {
			h.logExecution(r.Context(), webhookID, http.StatusBadRequest, string(bodyBytes), "", false,
				fmt.Sprintf("Template transformation failed: %v", err))
			writeError(w, http.StatusBadRequest, "template_error", fmt.Sprintf("Template transformation failed: %v", err))
			return
		}
		finalContent = result.Content
	}

	if finalContent == "" {
		h.logExecution(r.Context(), webhookID, http.StatusBadRequest, string(bodyBytes), "", false, "Message content cannot be empty")
		writeError(w, http.StatusBadRequest, "empty_content", "Message content cannot be empty")
		return
	}
	if len(finalContent) > 4000 {
		h.logExecution(r.Context(), webhookID, http.StatusBadRequest, string(bodyBytes), "", false, "Message content exceeds 4000 characters")
		writeError(w, http.StatusBadRequest, "content_too_long", "Message content exceeds 4000 characters")
		return
	}

	// Create the message.
	messageID := models.NewULID().String()
	now := time.Now().UTC()

	// Determine the display name: use override if provided, otherwise webhook name.
	displayName := wh.Name
	if req.Username != nil && *req.Username != "" {
		displayName = *req.Username
	}

	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO messages (id, channel_id, author_id, content, message_type, created_at)
		 VALUES ($1, $2, NULL, $3, 'webhook', $4)`,
		messageID, wh.ChannelID, finalContent, now)
	if err != nil {
		h.Logger.Error("failed to create webhook message",
			slog.String("error", err.Error()),
			slog.String("webhook_id", webhookID),
		)
		h.logExecution(r.Context(), webhookID, http.StatusInternalServerError, string(bodyBytes), "", false, "Failed to create message")
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create message")
		return
	}

	// Update channel last_message_id.
	h.Pool.Exec(r.Context(),
		`UPDATE channels SET last_message_id = $1 WHERE id = $2`, messageID, wh.ChannelID)

	// Publish message create event.
	h.EventBus.PublishJSON(r.Context(), events.SubjectMessageCreate, "MESSAGE_CREATE",
		map[string]interface{}{
			"id":           messageID,
			"channel_id":   wh.ChannelID,
			"guild_id":     wh.GuildID,
			"content":      finalContent,
			"webhook_id":   webhookID,
			"display_name": displayName,
			"avatar_url":   req.AvatarURL,
			"created_at":   now,
		})

	// Log successful execution.
	respData := map[string]interface{}{
		"id":         messageID,
		"channel_id": wh.ChannelID,
		"content":    finalContent,
		"webhook_id": webhookID,
		"author": map[string]interface{}{
			"id":       webhookID,
			"username": displayName,
			"bot":      true,
		},
		"created_at": now,
	}
	respBytes, _ := json.Marshal(respData)
	h.logExecution(r.Context(), webhookID, http.StatusOK, string(bodyBytes), string(respBytes), true, "")

	writeJSON(w, http.StatusOK, respData)
}

// HandleGetWebhookTemplates returns the list of built-in webhook templates.
// GET /api/v1/webhooks/templates
func (h *Handler) HandleGetWebhookTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, builtinTemplates)
}

// HandlePreviewWebhookMessage accepts a payload + template_id and returns the
// formatted message content without actually sending it.
// POST /api/v1/webhooks/preview
func (h *Handler) HandlePreviewWebhookMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TemplateID string          `json:"template_id"`
		Payload    json.RawMessage `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.TemplateID == "" {
		writeError(w, http.StatusBadRequest, "missing_template_id", "template_id is required")
		return
	}

	if _, exists := templatesByID[req.TemplateID]; !exists {
		writeError(w, http.StatusBadRequest, "unknown_template", "Unknown template: "+req.TemplateID)
		return
	}

	if len(req.Payload) == 0 {
		writeError(w, http.StatusBadRequest, "missing_payload", "payload is required")
		return
	}

	result, err := transformPayload(req.TemplateID, req.Payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, "transform_error", fmt.Sprintf("Failed to transform payload: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"content":     result.Content,
		"embeds":      result.Embeds,
		"template_id": req.TemplateID,
	})
}

// HandleGetWebhookLogs returns the execution logs for a specific webhook.
// GET /api/v1/guilds/{guildID}/webhooks/{webhookID}/logs
func (h *Handler) HandleGetWebhookLogs(w http.ResponseWriter, r *http.Request) {
	webhookID := chi.URLParam(r, "webhookID")
	guildID := chi.URLParam(r, "guildID")

	// Verify the webhook belongs to this guild.
	var whGuildID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id FROM webhooks WHERE id = $1`, webhookID,
	).Scan(&whGuildID)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "webhook_not_found", "Webhook not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to look up webhook")
		return
	}
	if whGuildID != guildID {
		writeError(w, http.StatusNotFound, "webhook_not_found", "Webhook not found in this guild")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, webhook_id, status_code, request_body, response_preview, success, error_message, created_at
		 FROM webhook_execution_logs
		 WHERE webhook_id = $1
		 ORDER BY created_at DESC
		 LIMIT 50`,
		webhookID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get webhook logs")
		return
	}
	defer rows.Close()

	logs := make([]WebhookExecutionLog, 0)
	for rows.Next() {
		var log WebhookExecutionLog
		if err := rows.Scan(
			&log.ID, &log.WebhookID, &log.StatusCode, &log.RequestBody,
			&log.ResponsePreview, &log.Success, &log.ErrorMessage, &log.CreatedAt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read webhook logs")
			return
		}
		logs = append(logs, log)
	}

	writeJSON(w, http.StatusOK, logs)
}

// HandleGetOutgoingEvents returns the list of valid outgoing event types.
// GET /api/v1/webhooks/outgoing-events
func (h *Handler) HandleGetOutgoingEvents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, ValidOutgoingEvents())
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{"code": code, "message": message},
	})
}
