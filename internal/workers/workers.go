// Package workers implements background job processing for tasks such as embed
// unfurling, media transcoding, expired session cleanup, search index sync,
// and federation message delivery retry. Workers consume events from NATS and
// run periodic maintenance tasks.
package workers

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/automod"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/notifications"
	"github.com/amityvox/amityvox/internal/search"
)

// Manager coordinates background workers and periodic jobs.
type Manager struct {
	pool          *pgxpool.Pool
	bus           *events.Bus
	search        *search.Service
	automod       *automod.Service
	notifications *notifications.Service
	logger        *slog.Logger
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// Config holds the configuration for the worker manager.
type Config struct {
	Pool          *pgxpool.Pool
	Bus           *events.Bus
	Search        *search.Service        // nil if search is disabled
	AutoMod       *automod.Service       // nil if automod is disabled
	Notifications *notifications.Service // nil if push is disabled
	Logger        *slog.Logger
}

// New creates a new worker manager.
func New(cfg Config) *Manager {
	return &Manager{
		pool:          cfg.Pool,
		bus:           cfg.Bus,
		search:        cfg.Search,
		automod:       cfg.AutoMod,
		notifications: cfg.Notifications,
		logger:        cfg.Logger,
	}
}

// Start launches all background workers. Call Stop() to shut them down.
func (m *Manager) Start(ctx context.Context) {
	ctx, m.cancel = context.WithCancel(ctx)

	// Start periodic cleanup workers.
	m.startPeriodic(ctx, "session-cleanup", 1*time.Hour, m.cleanExpiredSessions)
	m.startPeriodic(ctx, "invite-cleanup", 6*time.Hour, m.cleanExpiredInvites)

	// Start search sync worker if search is enabled.
	if m.search != nil {
		m.startPeriodic(ctx, "search-sync", 5*time.Minute, m.syncSearchIndex)
		m.startEventWorker(ctx)
	}

	// Start media workers (transcode + embed unfurling).
	m.startTranscodeWorker(ctx)
	m.startEmbedWorker(ctx)

	// Start automod worker (message content evaluation).
	if m.automod != nil {
		m.startAutomodWorker(ctx)
	}

	// Start push notification worker if enabled.
	if m.notifications != nil && m.notifications.Enabled() {
		m.startNotificationWorker(ctx)
		m.startEventReminderWorker(ctx)
		m.startPeriodic(ctx, "push-sub-cleanup", 24*time.Hour, m.cleanStalePushSubscriptions)
	}

	// Periodic MLS key package cleanup.
	m.startPeriodic(ctx, "mls-key-cleanup", 6*time.Hour, m.cleanExpiredKeyPackages)

	m.logger.Info("background workers started")
}

// Stop gracefully shuts down all workers and waits for them to finish.
func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
	m.logger.Info("background workers stopped")
}

// startPeriodic launches a goroutine that runs fn at the given interval.
func (m *Manager) startPeriodic(ctx context.Context, name string, interval time.Duration, fn func(context.Context) error) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		if err := fn(ctx); err != nil {
			m.logger.Error("worker error",
				slog.String("worker", name),
				slog.String("error", err.Error()),
			)
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := fn(ctx); err != nil {
					m.logger.Error("worker error",
						slog.String("worker", name),
						slog.String("error", err.Error()),
					)
				}
			}
		}
	}()
}

// startEventWorker subscribes to NATS message events and indexes them in Meilisearch.
func (m *Manager) startEventWorker(ctx context.Context) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		_, err := m.bus.SubscribeWildcard("amityvox.message.>", func(_ string, event events.Event) {
			switch event.Type {
			case "MESSAGE_CREATE":
				m.handleMessageCreate(ctx, event)
			case "MESSAGE_UPDATE":
				m.handleMessageUpdate(ctx, event)
			case "MESSAGE_DELETE":
				m.handleMessageDelete(ctx, event)
			}
		})
		if err != nil {
			m.logger.Error("failed to subscribe for search indexing",
				slog.String("error", err.Error()))
			return
		}

		<-ctx.Done()
	}()
}

// --- Periodic Job Implementations ---

func (m *Manager) cleanStalePushSubscriptions(ctx context.Context) error {
	return m.notifications.CleanupStaleSubscriptions(ctx, 90*24*time.Hour) // 90 days
}

func (m *Manager) cleanExpiredSessions(ctx context.Context) error {
	tag, err := m.pool.Exec(ctx,
		`DELETE FROM user_sessions WHERE expires_at < NOW()`)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		m.logger.Info("cleaned expired sessions",
			slog.Int64("deleted", tag.RowsAffected()))
	}
	return nil
}

func (m *Manager) cleanExpiredInvites(ctx context.Context) error {
	tag, err := m.pool.Exec(ctx,
		`DELETE FROM invites WHERE expires_at IS NOT NULL AND expires_at < NOW()`)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		m.logger.Info("cleaned expired invites",
			slog.Int64("deleted", tag.RowsAffected()))
	}
	return nil
}

func (m *Manager) syncSearchIndex(ctx context.Context) error {
	if m.search == nil {
		return nil
	}
	since := time.Now().Add(-10 * time.Minute)
	count, err := m.search.SyncMessages(ctx, since)
	if err != nil {
		return err
	}
	if count > 0 {
		m.logger.Debug("incremental search sync", slog.Int("indexed", count))
	}
	return nil
}

// --- Event Handler Implementations ---

func eventData(event events.Event) map[string]interface{} {
	var data map[string]interface{}
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return nil
	}
	return data
}

func (m *Manager) handleMessageCreate(ctx context.Context, event events.Event) {
	data := eventData(event)
	if data == nil {
		return
	}

	id, _ := data["id"].(string)
	channelID, _ := data["channel_id"].(string)
	guildID, _ := data["guild_id"].(string)
	authorID, _ := data["author_id"].(string)
	content, _ := data["content"].(string)

	if id == "" || content == "" {
		return
	}

	doc := search.MessageDoc{
		ID:        id,
		ChannelID: channelID,
		GuildID:   guildID,
		AuthorID:  authorID,
		Content:   content,
		CreatedAt: time.Now().Unix(),
	}

	if err := m.search.IndexMessage(ctx, doc); err != nil {
		m.logger.Error("failed to index message",
			slog.String("id", id),
			slog.String("error", err.Error()),
		)
	}
}

func (m *Manager) handleMessageUpdate(ctx context.Context, event events.Event) {
	data := eventData(event)
	if data == nil {
		return
	}

	id, _ := data["id"].(string)
	content, _ := data["content"].(string)

	if id == "" {
		return
	}

	var doc search.MessageDoc
	var guildID *string
	var msgContent *string
	var createdAt time.Time
	err := m.pool.QueryRow(ctx,
		`SELECT m.id, m.channel_id, c.guild_id, m.author_id, m.content, m.created_at
		 FROM messages m
		 LEFT JOIN channels c ON c.id = m.channel_id
		 WHERE m.id = $1`, id).Scan(
		&doc.ID, &doc.ChannelID, &guildID, &doc.AuthorID, &msgContent, &createdAt,
	)
	if err != nil {
		if content != "" {
			doc.ID = id
			doc.Content = content
			doc.CreatedAt = time.Now().Unix()
			m.search.IndexMessage(ctx, doc)
		}
		return
	}

	if guildID != nil {
		doc.GuildID = *guildID
	}
	if msgContent != nil {
		doc.Content = *msgContent
	}
	doc.CreatedAt = createdAt.Unix()
	m.search.IndexMessage(ctx, doc)
}

func (m *Manager) handleMessageDelete(ctx context.Context, event events.Event) {
	data := eventData(event)
	if data == nil {
		return
	}

	id, _ := data["id"].(string)
	if id == "" {
		return
	}

	if err := m.search.DeleteMessage(ctx, id); err != nil {
		m.logger.Error("failed to delete message from index",
			slog.String("id", id),
			slog.String("error", err.Error()),
		)
	}
}
