package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/mentions"
	"github.com/amityvox/amityvox/internal/models"
)

type dueScheduledMessage struct {
	ID            string
	ChannelID     string
	AuthorID      string
	Content       *string
	AttachmentIDs []string
}

func (m *Manager) deliverDueScheduledMessages(ctx context.Context) error {
	rows, err := m.pool.Query(ctx,
		`SELECT id, channel_id, author_id, content, attachment_ids
		 FROM scheduled_messages
		 WHERE scheduled_for <= now()
		 ORDER BY scheduled_for ASC
		 LIMIT 100`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var due []dueScheduledMessage
	for rows.Next() {
		var msg dueScheduledMessage
		if err := rows.Scan(&msg.ID, &msg.ChannelID, &msg.AuthorID, &msg.Content, &msg.AttachmentIDs); err != nil {
			return err
		}
		due = append(due, msg)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, msg := range due {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := m.deliverScheduledMessage(ctx, msg); err != nil {
			m.logger.Error("failed to deliver scheduled message",
				slog.String("scheduled_message_id", msg.ID),
				slog.String("channel_id", msg.ChannelID),
				slog.String("error", err.Error()),
			)
		}
	}

	return nil
}

func (m *Manager) deliverScheduledMessage(ctx context.Context, scheduled dueScheduledMessage) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var content *string
	var attachmentIDs []string
	if err := tx.QueryRow(ctx,
		`DELETE FROM scheduled_messages
		 WHERE id = $1 AND scheduled_for <= now()
		 RETURNING content, attachment_ids`,
		scheduled.ID,
	).Scan(&content, &attachmentIDs); err != nil {
		return err
	}

	var mentionUserIDs []string
	var mentionRoleIDs []string
	var mentionHere bool
	if content != nil {
		parsed := mentions.Parse(*content)
		mentionUserIDs = parsed.UserIDs
		mentionRoleIDs = parsed.RoleIDs
		mentionHere = parsed.MentionHere
	}

	now := time.Now().UTC()
	var msg models.Message
	if err := tx.QueryRow(ctx,
		`INSERT INTO messages (id, channel_id, author_id, content, message_type,
		                       mention_user_ids, mention_role_ids, mention_here, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id, channel_id, author_id, content, nonce, message_type, edited_at, flags,
		           reply_to_ids, mention_user_ids, mention_role_ids, mention_here,
		           thread_id, masquerade_name, masquerade_avatar, masquerade_color,
		           encrypted, encryption_session_id, expires_at, created_at`,
		scheduled.ID, scheduled.ChannelID, scheduled.AuthorID, content, models.MessageTypeDefault,
		mentionUserIDs, mentionRoleIDs, mentionHere, now,
	).Scan(
		&msg.ID, &msg.ChannelID, &msg.AuthorID, &msg.Content, &msg.Nonce, &msg.MessageType,
		&msg.EditedAt, &msg.Flags, &msg.ReplyToIDs, &msg.MentionUserIDs, &msg.MentionRoleIDs,
		&msg.MentionHere, &msg.ThreadID, &msg.MasqueradeName, &msg.MasqueradeAvatar,
		&msg.MasqueradeColor, &msg.Encrypted, &msg.EncryptionSessionID, &msg.ExpiresAt, &msg.CreatedAt,
	); err != nil {
		return err
	}

	if len(attachmentIDs) > 0 {
		tag, err := tx.Exec(ctx,
			`UPDATE attachments SET message_id = $1
			 WHERE id = ANY($2) AND uploader_id = $3 AND message_id IS NULL`,
			scheduled.ID, attachmentIDs, scheduled.AuthorID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != int64(len(attachmentIDs)) {
			return fmt.Errorf("scheduled message has invalid or already-linked attachments")
		}
	}

	if _, err := tx.Exec(ctx,
		`UPDATE channels SET last_message_id = $1 WHERE id = $2`,
		scheduled.ID, scheduled.ChannelID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE channels SET last_activity_at = now(), reply_count = reply_count + 1
		 WHERE id = $1 AND parent_channel_id IS NOT NULL`,
		scheduled.ChannelID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	msg.Attachments = m.loadMessageAttachments(ctx, msg.ID)
	payload, _ := json.Marshal(msg)
	return m.bus.Publish(ctx, events.SubjectMessageCreate, events.Event{
		Type:      "MESSAGE_CREATE",
		ChannelID: msg.ChannelID,
		Data:      payload,
	})
}

func (m *Manager) deleteExpiredMessages(ctx context.Context) error {
	rows, err := m.pool.Query(ctx,
		`DELETE FROM messages
		 WHERE id IN (
		     SELECT id FROM messages
		     WHERE expires_at IS NOT NULL AND expires_at <= now()
		     ORDER BY expires_at ASC
		     LIMIT 500
		 )
		 RETURNING id, channel_id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	byChannel := map[string][]string{}
	for rows.Next() {
		var messageID, channelID string
		if err := rows.Scan(&messageID, &channelID); err != nil {
			return err
		}
		byChannel[channelID] = append(byChannel[channelID], messageID)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for channelID, ids := range byChannel {
		if err := m.bus.PublishChannelEvent(ctx, events.SubjectMessageDeleteBulk, "MESSAGE_DELETE_BULK", channelID, map[string]interface{}{
			"channel_id":    channelID,
			"message_ids":   ids,
			"expired":       true,
			"deleted_count": len(ids),
		}); err != nil {
			m.logger.Warn("failed to publish expired message delete event",
				slog.String("channel_id", channelID),
				slog.String("error", err.Error()),
			)
		}
		if m.search != nil {
			for _, id := range ids {
				m.search.DeleteMessage(ctx, id)
			}
		}
	}

	return nil
}

func (m *Manager) loadMessageAttachments(ctx context.Context, messageID string) []models.Attachment {
	rows, err := m.pool.Query(ctx,
		`SELECT id, message_id, uploader_id, filename, content_type, size_bytes,
		        width, height, duration_seconds, s3_bucket, s3_key, blurhash, alt_text, created_at
		 FROM attachments WHERE message_id = $1
		 ORDER BY created_at`,
		messageID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	attachments := make([]models.Attachment, 0)
	for rows.Next() {
		var a models.Attachment
		if err := rows.Scan(
			&a.ID, &a.MessageID, &a.UploaderID, &a.Filename, &a.ContentType, &a.SizeBytes,
			&a.Width, &a.Height, &a.DurationSeconds, &a.S3Bucket, &a.S3Key, &a.Blurhash, &a.AltText, &a.CreatedAt,
		); err != nil {
			return nil
		}
		attachments = append(attachments, a)
	}
	return attachments
}
