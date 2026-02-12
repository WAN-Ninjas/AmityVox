// Package amityvox provides a Go SDK for building bots that interact with the
// AmityVox REST API and WebSocket gateway. It handles authentication, message
// sending/receiving, guild member management, and real-time event streaming.
//
// Basic usage:
//
//	bot := amityvox.NewBot("bot-token", "https://amityvox.example.com")
//	bot.OnMessage(func(msg *amityvox.Message) {
//	    if msg.Content != nil && *msg.Content == "!ping" {
//	        bot.Client().SendMessage(msg.ChannelID, "Pong!")
//	    }
//	})
//	bot.Start()
package amityvox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is the REST API client for the AmityVox API.
type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
	userAgent  string
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithHTTPClient sets a custom HTTP client for the API client.
func WithHTTPClient(c *http.Client) ClientOption {
	return func(cl *Client) {
		cl.httpClient = c
	}
}

// WithUserAgent sets a custom User-Agent header.
func WithUserAgent(ua string) ClientOption {
	return func(cl *Client) {
		cl.userAgent = ua
	}
}

// NewClient creates a new REST API client for AmityVox.
// The baseURL should be the root URL of the instance (e.g., "https://amityvox.example.com").
// The token is the bot token obtained from the bot management page.
func NewClient(token, baseURL string, opts ...ClientOption) *Client {
	// Ensure baseURL doesn't have a trailing slash.
	baseURL = strings.TrimRight(baseURL, "/")

	c := &Client{
		token:   token,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		userAgent: "AmityVox-Bot-SDK/1.0 (Go)",
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// apiResponse is the standard success envelope from the API.
type apiResponse struct {
	Data json.RawMessage `json:"data"`
}

// apiError is the standard error envelope from the API.
type apiError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// APIError represents an error response from the AmityVox API.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error %d (%s): %s", e.StatusCode, e.Code, e.Message)
}

// request performs an HTTP request to the API and decodes the response.
func (c *Client) request(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	u := c.baseURL + "/api/v1" + path

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending request to %s: %w", path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr apiError
		if err := json.Unmarshal(respBody, &apiErr); err == nil && apiErr.Error.Code != "" {
			return &APIError{
				StatusCode: resp.StatusCode,
				Code:       apiErr.Error.Code,
				Message:    apiErr.Error.Message,
			}
		}
		return &APIError{
			StatusCode: resp.StatusCode,
			Code:       "unknown_error",
			Message:    string(respBody),
		}
	}

	if result != nil && resp.StatusCode != http.StatusNoContent {
		var envelope apiResponse
		if err := json.Unmarshal(respBody, &envelope); err != nil {
			return fmt.Errorf("decoding response envelope: %w", err)
		}
		if err := json.Unmarshal(envelope.Data, result); err != nil {
			return fmt.Errorf("decoding response data: %w", err)
		}
	}

	return nil
}

// --- User Endpoints ---

// GetSelf returns the authenticated bot user.
func (c *Client) GetSelf(ctx context.Context) (*User, error) {
	var user User
	if err := c.request(ctx, http.MethodGet, "/users/@me", nil, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUser returns a user by ID.
func (c *Client) GetUser(ctx context.Context, userID string) (*User, error) {
	var user User
	if err := c.request(ctx, http.MethodGet, "/users/"+userID, nil, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// --- Message Endpoints ---

// SendMessage sends a text message to a channel.
func (c *Client) SendMessage(ctx context.Context, channelID, content string) (*Message, error) {
	body := map[string]interface{}{
		"content": content,
	}
	var msg Message
	if err := c.request(ctx, http.MethodPost, "/channels/"+channelID+"/messages", body, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// SendMessageWithEmbed sends a message with an embed to a channel.
func (c *Client) SendMessageWithEmbed(ctx context.Context, channelID, content string, embeds []MessageEmbed) (*Message, error) {
	body := map[string]interface{}{
		"content": content,
		"embeds":  embeds,
	}
	var msg Message
	if err := c.request(ctx, http.MethodPost, "/channels/"+channelID+"/messages", body, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// EditMessage edits an existing message.
func (c *Client) EditMessage(ctx context.Context, channelID, messageID, content string) (*Message, error) {
	body := map[string]string{"content": content}
	var msg Message
	if err := c.request(ctx, http.MethodPatch, "/channels/"+channelID+"/messages/"+messageID, body, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// DeleteMessage deletes a message.
func (c *Client) DeleteMessage(ctx context.Context, channelID, messageID string) error {
	return c.request(ctx, http.MethodDelete, "/channels/"+channelID+"/messages/"+messageID, nil, nil)
}

// GetMessages fetches messages from a channel. Supports pagination with before/after/limit.
func (c *Client) GetMessages(ctx context.Context, channelID string, opts *GetMessagesOpts) ([]Message, error) {
	path := "/channels/" + channelID + "/messages"
	if opts != nil {
		params := url.Values{}
		if opts.Before != "" {
			params.Set("before", opts.Before)
		}
		if opts.After != "" {
			params.Set("after", opts.After)
		}
		if opts.Limit > 0 {
			params.Set("limit", fmt.Sprintf("%d", opts.Limit))
		}
		if len(params) > 0 {
			path += "?" + params.Encode()
		}
	}
	var msgs []Message
	if err := c.request(ctx, http.MethodGet, path, nil, &msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

// GetMessagesOpts are optional parameters for GetMessages.
type GetMessagesOpts struct {
	Before string
	After  string
	Limit  int
}

// --- Reaction Endpoints ---

// AddReaction adds an emoji reaction to a message.
func (c *Client) AddReaction(ctx context.Context, channelID, messageID, emoji string) error {
	return c.request(ctx, http.MethodPut,
		"/channels/"+channelID+"/messages/"+messageID+"/reactions/"+url.PathEscape(emoji),
		nil, nil)
}

// RemoveReaction removes the bot's reaction from a message.
func (c *Client) RemoveReaction(ctx context.Context, channelID, messageID, emoji string) error {
	return c.request(ctx, http.MethodDelete,
		"/channels/"+channelID+"/messages/"+messageID+"/reactions/"+url.PathEscape(emoji),
		nil, nil)
}

// --- Channel Endpoints ---

// GetChannel returns a channel by ID.
func (c *Client) GetChannel(ctx context.Context, channelID string) (*Channel, error) {
	var ch Channel
	if err := c.request(ctx, http.MethodGet, "/channels/"+channelID, nil, &ch); err != nil {
		return nil, err
	}
	return &ch, nil
}

// TriggerTyping sends a typing indicator to a channel.
func (c *Client) TriggerTyping(ctx context.Context, channelID string) error {
	return c.request(ctx, http.MethodPost, "/channels/"+channelID+"/typing", nil, nil)
}

// --- Guild Endpoints ---

// GetGuild returns a guild by ID.
func (c *Client) GetGuild(ctx context.Context, guildID string) (*Guild, error) {
	var guild Guild
	if err := c.request(ctx, http.MethodGet, "/guilds/"+guildID, nil, &guild); err != nil {
		return nil, err
	}
	return &guild, nil
}

// GetGuildChannels returns all channels in a guild.
func (c *Client) GetGuildChannels(ctx context.Context, guildID string) ([]Channel, error) {
	var channels []Channel
	if err := c.request(ctx, http.MethodGet, "/guilds/"+guildID+"/channels", nil, &channels); err != nil {
		return nil, err
	}
	return channels, nil
}

// GetGuildMembers returns members of a guild.
func (c *Client) GetGuildMembers(ctx context.Context, guildID string) ([]GuildMember, error) {
	var members []GuildMember
	if err := c.request(ctx, http.MethodGet, "/guilds/"+guildID+"/members", nil, &members); err != nil {
		return nil, err
	}
	return members, nil
}

// GetGuildMember returns a specific member of a guild.
func (c *Client) GetGuildMember(ctx context.Context, guildID, userID string) (*GuildMember, error) {
	var member GuildMember
	if err := c.request(ctx, http.MethodGet, "/guilds/"+guildID+"/members/"+userID, nil, &member); err != nil {
		return nil, err
	}
	return &member, nil
}

// AddMemberRole adds a role to a guild member.
func (c *Client) AddMemberRole(ctx context.Context, guildID, userID, roleID string) error {
	return c.request(ctx, http.MethodPut,
		"/guilds/"+guildID+"/members/"+userID+"/roles/"+roleID, nil, nil)
}

// RemoveMemberRole removes a role from a guild member.
func (c *Client) RemoveMemberRole(ctx context.Context, guildID, userID, roleID string) error {
	return c.request(ctx, http.MethodDelete,
		"/guilds/"+guildID+"/members/"+userID+"/roles/"+roleID, nil, nil)
}

// GetGuildRoles returns all roles in a guild.
func (c *Client) GetGuildRoles(ctx context.Context, guildID string) ([]Role, error) {
	var roles []Role
	if err := c.request(ctx, http.MethodGet, "/guilds/"+guildID+"/roles", nil, &roles); err != nil {
		return nil, err
	}
	return roles, nil
}

// --- Invite Endpoints ---

// GetInvite returns an invite by code.
func (c *Client) GetInvite(ctx context.Context, code string) (*Invite, error) {
	var invite Invite
	if err := c.request(ctx, http.MethodGet, "/invites/"+code, nil, &invite); err != nil {
		return nil, err
	}
	return &invite, nil
}

// Token returns the current authentication token.
func (c *Client) Token() string {
	return c.token
}

// BaseURL returns the configured base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}
