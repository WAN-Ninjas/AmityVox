// Package search integrates with Meilisearch to provide full-text search across
// messages, users, guilds, and channels. It handles index management, document
// synchronization, and search query execution.
package search

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/meilisearch/meilisearch-go"
)

// Index names for the Meilisearch collections.
const (
	IndexMessages = "messages"
	IndexUsers    = "users"
	IndexGuilds   = "guilds"
	IndexChannels = "channels"
)

// Service provides full-text search operations backed by Meilisearch.
type Service struct {
	client *meilisearch.ServiceManager
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// Config holds the configuration for the search service.
type Config struct {
	URL    string
	APIKey string
	Pool   *pgxpool.Pool
	Logger *slog.Logger
}

// New creates a new search service connected to Meilisearch.
func New(cfg Config) (*Service, error) {
	client := meilisearch.New(cfg.URL, meilisearch.WithAPIKey(cfg.APIKey))
	return &Service{
		client: &client,
		pool:   cfg.Pool,
		logger: cfg.Logger,
	}, nil
}

// docOpts returns DocumentOptions with primary key "id".
func docOpts() *meilisearch.DocumentOptions {
	pk := "id"
	return &meilisearch.DocumentOptions{PrimaryKey: &pk}
}

// EnsureIndexes creates the Meilisearch indexes with proper settings if they
// don't already exist.
func (s *Service) EnsureIndexes(ctx context.Context) error {
	indexes := []struct {
		uid        string
		primaryKey string
		searchable []string
		filterable []string
		sortable   []string
	}{
		{
			uid:        IndexMessages,
			primaryKey: "id",
			searchable: []string{"content"},
			filterable: []string{"channel_id", "guild_id", "author_id", "created_at"},
			sortable:   []string{"created_at"},
		},
		{
			uid:        IndexUsers,
			primaryKey: "id",
			searchable: []string{"username", "display_name"},
			filterable: []string{"instance_id"},
			sortable:   []string{"username"},
		},
		{
			uid:        IndexGuilds,
			primaryKey: "id",
			searchable: []string{"name", "description"},
			filterable: []string{},
			sortable:   []string{"name", "member_count"},
		},
		{
			uid:        IndexChannels,
			primaryKey: "id",
			searchable: []string{"name", "topic"},
			filterable: []string{"guild_id", "channel_type"},
			sortable:   []string{"name", "position"},
		},
	}

	for _, idx := range indexes {
		task, err := (*s.client).CreateIndex(&meilisearch.IndexConfig{
			Uid:        idx.uid,
			PrimaryKey: idx.primaryKey,
		})
		if err != nil {
			s.logger.Debug("index creation (may already exist)",
				slog.String("index", idx.uid),
				slog.String("error", err.Error()),
			)
		} else {
			s.logger.Info("created search index",
				slog.String("index", idx.uid),
				slog.Int64("task_uid", task.TaskUID),
			)
		}

		index := (*s.client).Index(idx.uid)
		if len(idx.searchable) > 0 {
			index.UpdateSearchableAttributes(&idx.searchable)
		}
		if len(idx.filterable) > 0 {
			// Convert []string to []interface{} for the API.
			attrs := make([]interface{}, len(idx.filterable))
			for i, v := range idx.filterable {
				attrs[i] = v
			}
			index.UpdateFilterableAttributes(&attrs)
		}
		if len(idx.sortable) > 0 {
			index.UpdateSortableAttributes(&idx.sortable)
		}
	}

	return nil
}

// MessageDoc is the document format for messages indexed in Meilisearch.
type MessageDoc struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	GuildID   string `json:"guild_id,omitempty"`
	AuthorID  string `json:"author_id"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
}

// IndexMessage adds or updates a message in the search index.
func (s *Service) IndexMessage(ctx context.Context, doc MessageDoc) error {
	index := (*s.client).Index(IndexMessages)
	_, err := index.AddDocuments([]MessageDoc{doc}, docOpts())
	if err != nil {
		return fmt.Errorf("indexing message %s: %w", doc.ID, err)
	}
	return nil
}

// DeleteMessage removes a message from the search index.
func (s *Service) DeleteMessage(ctx context.Context, messageID string) error {
	index := (*s.client).Index(IndexMessages)
	_, err := index.DeleteDocument(messageID, nil)
	if err != nil {
		return fmt.Errorf("deleting message %s from index: %w", messageID, err)
	}
	return nil
}

// UserDoc is the document format for users indexed in Meilisearch.
type UserDoc struct {
	ID          string  `json:"id"`
	InstanceID  string  `json:"instance_id"`
	Username    string  `json:"username"`
	DisplayName *string `json:"display_name,omitempty"`
}

// IndexUser adds or updates a user in the search index.
func (s *Service) IndexUser(ctx context.Context, doc UserDoc) error {
	index := (*s.client).Index(IndexUsers)
	_, err := index.AddDocuments([]UserDoc{doc}, docOpts())
	if err != nil {
		return fmt.Errorf("indexing user %s: %w", doc.ID, err)
	}
	return nil
}

// GuildDoc is the document format for guilds indexed in Meilisearch.
type GuildDoc struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MemberCount int    `json:"member_count"`
}

// IndexGuild adds or updates a guild in the search index.
func (s *Service) IndexGuild(ctx context.Context, doc GuildDoc) error {
	index := (*s.client).Index(IndexGuilds)
	_, err := index.AddDocuments([]GuildDoc{doc}, docOpts())
	if err != nil {
		return fmt.Errorf("indexing guild %s: %w", doc.ID, err)
	}
	return nil
}

// SearchRequest defines parameters for a search query.
type SearchRequest struct {
	Query   string
	Index   string
	Filters string
	Limit   int64
	Offset  int64
}

// SearchResult holds results from a search query.
type SearchResult struct {
	IDs              []string `json:"ids"`
	EstimatedTotal   int64    `json:"estimated_total"`
	ProcessingTimeMs int64    `json:"processing_time_ms"`
}

// Search executes a full-text search query against the specified index.
// Returns the IDs of matching documents in relevance order.
func (s *Service) Search(ctx context.Context, req SearchRequest) (*SearchResult, error) {
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 20
	}

	index := (*s.client).Index(req.Index)

	searchReq := &meilisearch.SearchRequest{
		Limit:  req.Limit,
		Offset: req.Offset,
	}
	if req.Filters != "" {
		searchReq.Filter = req.Filters
	}

	resp, err := index.Search(req.Query, searchReq)
	if err != nil {
		return nil, fmt.Errorf("searching %s: %w", req.Index, err)
	}

	// Extract IDs from hits. Each hit is map[string]json.RawMessage.
	ids := make([]string, 0, len(resp.Hits))
	for _, hit := range resp.Hits {
		if raw, ok := hit["id"]; ok {
			var id string
			if err := json.Unmarshal(raw, &id); err == nil && id != "" {
				ids = append(ids, id)
			}
		}
	}

	return &SearchResult{
		IDs:              ids,
		EstimatedTotal:   resp.EstimatedTotalHits,
		ProcessingTimeMs: resp.ProcessingTimeMs,
	}, nil
}

// SyncMessages reindexes all messages from the database. Used for initial
// population or recovery. Should be run as a background job.
func (s *Service) SyncMessages(ctx context.Context, since time.Time) (int, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT m.id, m.channel_id, c.guild_id, m.author_id, m.content, m.created_at
		 FROM messages m
		 LEFT JOIN channels c ON c.id = m.channel_id
		 WHERE m.created_at > $1 AND m.content IS NOT NULL
		 ORDER BY m.created_at ASC
		 LIMIT 10000`, since)
	if err != nil {
		return 0, fmt.Errorf("querying messages for sync: %w", err)
	}
	defer rows.Close()

	var docs []MessageDoc
	for rows.Next() {
		var doc MessageDoc
		var guildID *string
		var content *string
		var createdAt time.Time
		if err := rows.Scan(&doc.ID, &doc.ChannelID, &guildID, &doc.AuthorID, &content, &createdAt); err != nil {
			return 0, fmt.Errorf("scanning message for sync: %w", err)
		}
		if content != nil {
			doc.Content = *content
		}
		if guildID != nil {
			doc.GuildID = *guildID
		}
		doc.CreatedAt = createdAt.Unix()
		docs = append(docs, doc)
	}

	if len(docs) == 0 {
		return 0, nil
	}

	index := (*s.client).Index(IndexMessages)
	_, err = index.AddDocuments(docs, docOpts())
	if err != nil {
		return 0, fmt.Errorf("batch indexing messages: %w", err)
	}

	s.logger.Info("synced messages to search index", slog.Int("count", len(docs)))
	return len(docs), nil
}

// HealthCheck verifies Meilisearch connectivity.
func (s *Service) HealthCheck() error {
	ok := (*s.client).IsHealthy()
	if !ok {
		return fmt.Errorf("meilisearch is not healthy")
	}
	return nil
}
