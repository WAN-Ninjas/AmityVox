package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/amityvox/amityvox/internal/events"
)

// runRetentionPolicies finds all enabled policies due for execution and
// processes each one independently. A failure in one policy does not stop others.
func (m *Manager) runRetentionPolicies(ctx context.Context) error {
	rows, err := m.pool.Query(ctx,
		`SELECT id, channel_id, guild_id, max_age_days, delete_attachments, delete_pins
		 FROM data_retention_policies
		 WHERE enabled = true AND (next_run_at IS NULL OR next_run_at <= NOW())`)
	if err != nil {
		return fmt.Errorf("querying retention policies: %w", err)
	}
	defer rows.Close()

	type policy struct {
		ID                string
		ChannelID         *string
		GuildID           *string
		MaxAgeDays        int
		DeleteAttachments bool
		DeletePins        bool
	}

	var policies []policy
	for rows.Next() {
		var p policy
		if err := rows.Scan(&p.ID, &p.ChannelID, &p.GuildID, &p.MaxAgeDays, &p.DeleteAttachments, &p.DeletePins); err != nil {
			m.logger.Error("scanning retention policy", slog.String("error", err.Error()))
			continue
		}
		policies = append(policies, p)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating retention policies: %w", err)
	}

	for _, p := range policies {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := m.executeRetentionPolicy(ctx, p.ID, p.ChannelID, p.GuildID, p.MaxAgeDays, p.DeleteAttachments, p.DeletePins); err != nil {
			m.logger.Error("retention policy execution failed",
				slog.String("policy_id", p.ID),
				slog.String("error", err.Error()),
			)
			// Continue with next policy — error isolation.
		}
	}

	return nil
}

// executeRetentionPolicy runs a single retention policy: batch-deletes messages
// older than the cutoff, optionally removes S3 attachments, and publishes bulk
// delete events.
func (m *Manager) executeRetentionPolicy(ctx context.Context, policyID string, channelID, guildID *string, maxAgeDays int, deleteAttachments, deletePins bool) error {
	cutoff := time.Now().UTC().Add(-time.Duration(maxAgeDays) * 24 * time.Hour)
	var totalDeleted int64

	const batchSize = 1000

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Find a batch of message IDs to delete.
		var query string
		var args []interface{}

		if channelID != nil {
			query = `SELECT m.id FROM messages m WHERE m.channel_id = $1 AND m.created_at < $2`
			args = []interface{}{*channelID, cutoff}
			if !deletePins {
				query += ` AND m.id NOT IN (SELECT p.message_id FROM pins p WHERE p.channel_id = $1)`
			}
		} else if guildID != nil {
			query = `SELECT m.id FROM messages m
			         JOIN channels c ON c.id = m.channel_id
			         WHERE c.guild_id = $1 AND m.created_at < $2`
			args = []interface{}{*guildID, cutoff}
			if !deletePins {
				query += ` AND m.id NOT IN (SELECT p.message_id FROM pins p)`
			}
		} else {
			return nil // No scope — skip.
		}

		query += fmt.Sprintf(` LIMIT %d`, batchSize)

		rows, err := m.pool.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("querying messages for deletion: %w", err)
		}

		var messageIDs []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return fmt.Errorf("scanning message id: %w", err)
			}
			messageIDs = append(messageIDs, id)
		}
		rows.Close()

		if len(messageIDs) == 0 {
			break // No more messages to delete.
		}

		// Delete S3 attachments if configured and media service available.
		if deleteAttachments && m.media != nil {
			attRows, err := m.pool.Query(ctx,
				`SELECT id, s3_bucket, s3_key FROM attachments WHERE message_id = ANY($1)`,
				messageIDs)
			if err == nil {
				type att struct {
					ID     string
					Bucket string
					Key    string
				}
				var atts []att
				for attRows.Next() {
					var a att
					if err := attRows.Scan(&a.ID, &a.Bucket, &a.Key); err == nil {
						atts = append(atts, a)
					}
				}
				attRows.Close()

				for _, a := range atts {
					if err := m.media.DeleteObject(ctx, a.Bucket, a.Key); err != nil {
						m.logger.Warn("failed to delete S3 object during retention cleanup",
							slog.String("key", a.Key),
							slog.String("error", err.Error()),
						)
					}
				}

				// Delete attachment rows.
				if len(atts) > 0 {
					attIDs := make([]string, len(atts))
					for i, a := range atts {
						attIDs[i] = a.ID
					}
					m.pool.Exec(ctx, `DELETE FROM attachments WHERE id = ANY($1)`, attIDs)
				}
			}
		}

		// Delete the messages.
		tag, err := m.pool.Exec(ctx,
			`DELETE FROM messages WHERE id = ANY($1)`, messageIDs)
		if err != nil {
			return fmt.Errorf("deleting message batch: %w", err)
		}
		totalDeleted += tag.RowsAffected()

		// Determine channel_id for the event. For channel-scoped, use it directly.
		// For guild-scoped, we emit a generic event with the guild context.
		eventChannelID := ""
		if channelID != nil {
			eventChannelID = *channelID
		}

		// Publish MESSAGE_DELETE_BULK event.
		eventData, _ := json.Marshal(map[string]interface{}{
			"ids":        messageIDs,
			"channel_id": eventChannelID,
			"guild_id":   guildID,
		})
		m.bus.Publish(ctx, events.SubjectMessageDelete, events.Event{
			Type:      "MESSAGE_DELETE_BULK",
			ChannelID: eventChannelID,
			Data:      eventData,
		})

		// Delete from search index.
		if m.search != nil {
			for _, id := range messageIDs {
				m.search.DeleteMessage(ctx, id)
			}
		}

		// If we got fewer than batchSize, we're done.
		if len(messageIDs) < batchSize {
			break
		}
	}

	// Update policy stats.
	_, err := m.pool.Exec(ctx,
		`UPDATE data_retention_policies
		 SET last_run_at = now(),
		     next_run_at = now() + INTERVAL '24 hours',
		     messages_deleted = messages_deleted + $1,
		     updated_at = now()
		 WHERE id = $2`, totalDeleted, policyID)
	if err != nil {
		return fmt.Errorf("updating policy stats: %w", err)
	}

	if totalDeleted > 0 {
		m.logger.Info("retention policy executed",
			slog.String("policy_id", policyID),
			slog.Int64("messages_deleted", totalDeleted),
		)
	}

	return nil
}
