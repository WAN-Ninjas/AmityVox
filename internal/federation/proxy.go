package federation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

// ForwardToHomeInstance sends a signed management request to the guild's home
// instance. It looks up the instance domain, builds a manageRequest envelope,
// signs it with Ed25519, and POSTs to the remote /federation/v1/guilds/{guildID}/manage
// endpoint. Returns the parsed manageResponse, or an error if the request failed.
func (ss *SyncService) ForwardToHomeInstance(
	ctx context.Context,
	guildID string,
	instanceID string,
	action string,
	userID string,
	data interface{},
) (*manageResponse, error) {
	// 1. Look up the home instance's domain.
	var domain string
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT domain FROM instances WHERE id = $1`, instanceID,
	).Scan(&domain)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("home instance %s not found", instanceID)
		}
		return nil, fmt.Errorf("looking up home instance %s: %w", instanceID, err)
	}

	// 2. Marshal the action data into json.RawMessage for the manage request.
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshaling action data: %w", err)
	}

	innerPayload := manageRequest{
		Action: action,
		UserID: userID,
		Data:   json.RawMessage(dataJSON),
	}

	// 3. Sign the inner payload.
	signed, err := ss.fed.Sign(innerPayload)
	if err != nil {
		return nil, fmt.Errorf("signing management request: %w", err)
	}

	// 4. POST to the remote manage endpoint with a 5-second timeout.
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	url := fmt.Sprintf("https://%s/federation/v1/guilds/%s/manage", domain, guildID)

	body, err := json.Marshal(signed)
	if err != nil {
		return nil, fmt.Errorf("marshaling signed payload: %w", err)
	}

	req, err := http.NewRequestWithContext(reqCtx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating management request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "AmityVox/1.0 (+federation)")

	resp, err := ss.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending management request to %s: %w", domain, err)
	}
	defer resp.Body.Close()

	// 5. Read and parse the response body.
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", domain, err)
	}

	var mresp manageResponse
	if err := json.Unmarshal(respBody, &mresp); err != nil {
		return nil, fmt.Errorf("decoding management response from %s (status %d): %w", domain, resp.StatusCode, err)
	}

	return &mresp, nil
}

// ProxyToHomeInstance is a convenience wrapper for HTTP handlers. It checks if
// the guild is federated (instanceID is non-nil), and if so, forwards the
// management request to the home instance and writes the response back to the
// caller. Returns true if the request was forwarded (the handler should return
// immediately), or false if the guild is local (the handler should continue
// with its normal local logic).
func (ss *SyncService) ProxyToHomeInstance(
	w http.ResponseWriter,
	r *http.Request,
	guildID string,
	instanceID *string,
	action string,
	userID string,
	data interface{},
) bool {
	// Local guild — handler should continue with local logic.
	if instanceID == nil {
		return false
	}

	resp, err := ss.ForwardToHomeInstance(r.Context(), guildID, *instanceID, action, userID, data)
	if err != nil {
		ss.logger.Error("failed to forward management request to home instance",
			slog.String("guild_id", guildID),
			slog.String("instance_id", *instanceID),
			slog.String("action", action),
			slog.String("error", err.Error()))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"code":    "FEDERATION_PROXY_ERROR",
				"message": "Failed to forward request to home instance",
			},
		})
		return true
	}

	w.Header().Set("Content-Type", "application/json")

	if !resp.OK {
		// The remote instance returned an error — pass it through.
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"code":    "FEDERATION_REMOTE_ERROR",
				"message": resp.Error,
			},
		})
		return true
	}

	// Success — write the remote response data as the response body.
	if resp.Data != nil {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]json.RawMessage{
			"data": resp.Data,
		})
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
	return true
}
