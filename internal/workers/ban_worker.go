package workers

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/amityvox/amityvox/internal/events"
)

func (m *Manager) cleanExpiredBans(ctx context.Context) error {
	rows, err := m.pool.Query(ctx,
		`DELETE FROM guild_bans
		 WHERE expires_at IS NOT NULL AND expires_at < NOW()
		 RETURNING guild_id, user_id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var count int64
	for rows.Next() {
		var guildID, userID string
		if err := rows.Scan(&guildID, &userID); err != nil {
			m.logger.Error("failed to scan expired ban row", slog.String("error", err.Error()))
			continue
		}
		m.bus.PublishGuildEvent(ctx, events.SubjectGuildBanRemove, "GUILD_BAN_REMOVE", guildID, map[string]string{
			"guild_id": guildID, "user_id": userID,
		})
		count++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating expired bans: %w", err)
	}
	if count > 0 {
		m.logger.Info("cleaned expired bans",
			slog.Int64("removed", count))
	}
	return nil
}
