// Package integration provides integration tests for AmityVox using dockertest.
// These tests spin up real PostgreSQL, NATS, and DragonflyDB containers, run
// migrations, and test the full stack including database queries, event bus
// pub/sub, and cache operations. Tests are skipped if Docker is unavailable.
//
// Run with: go test -tags integration ./internal/integration/ -v
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/database"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
	"github.com/amityvox/amityvox/internal/presence"
)

var (
	testPool   *pgxpool.Pool
	testDB     *database.DB
	testBus    *events.Bus
	testCache  *presence.Cache
	testLogger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	dockerPool *dockertest.Pool
)

// TestMain sets up Docker containers for integration testing.
func TestMain(m *testing.M) {
	// Check if Docker is available.
	pool, err := dockertest.NewPool("")
	if err != nil {
		fmt.Printf("Skipping integration tests: Docker not available: %v\n", err)
		os.Exit(0)
	}
	if err := pool.Client.Ping(); err != nil {
		fmt.Printf("Skipping integration tests: Docker not reachable: %v\n", err)
		os.Exit(0)
	}
	dockerPool = pool
	pool.MaxWait = 120 * time.Second

	// Start PostgreSQL.
	pgResource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "16-alpine",
		Env: []string{
			"POSTGRES_USER=amityvox_test",
			"POSTGRES_PASSWORD=testpass",
			"POSTGRES_DB=amityvox_test",
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		fmt.Printf("Could not start PostgreSQL: %v\n", err)
		os.Exit(1)
	}

	pgURL := fmt.Sprintf("postgres://amityvox_test:testpass@localhost:%s/amityvox_test?sslmode=disable",
		pgResource.GetPort("5432/tcp"))

	// Wait for PostgreSQL to be ready.
	if err := pool.Retry(func() error {
		ctx := context.Background()
		db, err := database.New(ctx, pgURL, 5, testLogger)
		if err != nil {
			return err
		}
		testDB = db
		testPool = db.Pool
		return db.HealthCheck(ctx)
	}); err != nil {
		fmt.Printf("Could not connect to PostgreSQL: %v\n", err)
		pgResource.Close()
		os.Exit(1)
	}

	// Run migrations.
	if err := database.MigrateUp(pgURL, testLogger); err != nil {
		fmt.Printf("Migration failed: %v\n", err)
		pgResource.Close()
		os.Exit(1)
	}

	// Start NATS.
	natsResource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "nats",
		Tag:        "2-alpine",
		Cmd:        []string{"-js"},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		fmt.Printf("Could not start NATS: %v\n", err)
		pgResource.Close()
		os.Exit(1)
	}

	natsURL := fmt.Sprintf("nats://localhost:%s", natsResource.GetPort("4222/tcp"))

	// Wait for NATS to be ready.
	if err := pool.Retry(func() error {
		bus, err := events.New(natsURL, testLogger)
		if err != nil {
			return err
		}
		testBus = bus
		return bus.HealthCheck()
	}); err != nil {
		fmt.Printf("Could not connect to NATS: %v\n", err)
		pgResource.Close()
		natsResource.Close()
		os.Exit(1)
	}

	// Start DragonflyDB (Redis-compatible).
	redisResource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "redis",
		Tag:        "7-alpine",
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		fmt.Printf("Could not start Redis: %v\n", err)
		pgResource.Close()
		natsResource.Close()
		os.Exit(1)
	}

	redisURL := fmt.Sprintf("redis://localhost:%s", redisResource.GetPort("6379/tcp"))

	// Wait for Redis to be ready.
	if err := pool.Retry(func() error {
		cache, err := presence.New(redisURL, testLogger)
		if err != nil {
			return err
		}
		testCache = cache
		return cache.HealthCheck(context.Background())
	}); err != nil {
		fmt.Printf("Could not connect to Redis: %v\n", err)
		pgResource.Close()
		natsResource.Close()
		redisResource.Close()
		os.Exit(1)
	}

	// Run tests.
	code := m.Run()

	// Cleanup.
	testDB.Close()
	testBus.Close()
	testCache.Close()
	pgResource.Close()
	natsResource.Close()
	redisResource.Close()

	os.Exit(code)
}

// --- Database Integration Tests ---

func TestDatabaseHealthCheck(t *testing.T) {
	if err := testDB.HealthCheck(context.Background()); err != nil {
		t.Fatalf("database health check failed: %v", err)
	}
}

func TestCreateAndQueryUser(t *testing.T) {
	ctx := context.Background()
	userID := models.NewULID().String()
	username := "integration_" + userID[:8]

	// Bootstrap instance if needed.
	var instanceID string
	err := testPool.QueryRow(ctx, `SELECT id FROM instances LIMIT 1`).Scan(&instanceID)
	if err != nil {
		instanceID = models.NewULID().String()
		_, err = testPool.Exec(ctx,
			`INSERT INTO instances (id, domain, public_key, name, software_version, federation_mode, created_at)
			 VALUES ($1, 'test.local', 'test-key', 'Test Instance', 'test', 'closed', now())`,
			instanceID)
		if err != nil {
			t.Fatalf("creating test instance: %v", err)
		}
	}

	// Create user.
	_, err = testPool.Exec(ctx,
		`INSERT INTO users (id, instance_id, username, password_hash, created_at)
		 VALUES ($1, $2, $3, 'test-hash', now())`,
		userID, instanceID, username)
	if err != nil {
		t.Fatalf("creating user: %v", err)
	}

	// Query user.
	var foundUsername string
	err = testPool.QueryRow(ctx,
		`SELECT username FROM users WHERE id = $1`, userID).Scan(&foundUsername)
	if err != nil {
		t.Fatalf("querying user: %v", err)
	}
	if foundUsername != username {
		t.Errorf("expected username %q, got %q", username, foundUsername)
	}

	// Clean up.
	testPool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
}

func TestCreateGuildAndChannel(t *testing.T) {
	ctx := context.Background()

	// Ensure instance + user exist.
	var instanceID string
	testPool.QueryRow(ctx, `SELECT id FROM instances LIMIT 1`).Scan(&instanceID)
	if instanceID == "" {
		instanceID = models.NewULID().String()
		testPool.Exec(ctx,
			`INSERT INTO instances (id, domain, public_key, name, software_version, federation_mode, created_at)
			 VALUES ($1, 'test.local', 'test-key', 'Test Instance', 'test', 'closed', now())`,
			instanceID)
	}

	userID := models.NewULID().String()
	testPool.Exec(ctx,
		`INSERT INTO users (id, instance_id, username, password_hash, created_at)
		 VALUES ($1, $2, $3, 'hash', now())`,
		userID, instanceID, "guild_test_"+userID[:6])

	// Create guild.
	guildID := models.NewULID().String()
	_, err := testPool.Exec(ctx,
		`INSERT INTO guilds (id, name, owner_id, created_at) VALUES ($1, $2, $3, now())`,
		guildID, "Test Guild", userID)
	if err != nil {
		t.Fatalf("creating guild: %v", err)
	}

	// Create channel in guild.
	channelID := models.NewULID().String()
	_, err = testPool.Exec(ctx,
		`INSERT INTO channels (id, guild_id, name, channel_type, position, created_at)
		 VALUES ($1, $2, $3, 'text', 0, now())`,
		channelID, guildID, "general")
	if err != nil {
		t.Fatalf("creating channel: %v", err)
	}

	// Verify.
	var channelName string
	testPool.QueryRow(ctx,
		`SELECT name FROM channels WHERE id = $1`, channelID).Scan(&channelName)
	if channelName != "general" {
		t.Errorf("expected channel name 'general', got %q", channelName)
	}

	// Create and query a message.
	msgID := models.NewULID().String()
	_, err = testPool.Exec(ctx,
		`INSERT INTO messages (id, channel_id, author_id, content, created_at)
		 VALUES ($1, $2, $3, $4, now())`,
		msgID, channelID, userID, "Hello integration test!")
	if err != nil {
		t.Fatalf("creating message: %v", err)
	}

	var content string
	testPool.QueryRow(ctx,
		`SELECT content FROM messages WHERE id = $1`, msgID).Scan(&content)
	if content != "Hello integration test!" {
		t.Errorf("expected message content 'Hello integration test!', got %q", content)
	}

	// Clean up.
	testPool.Exec(ctx, `DELETE FROM messages WHERE id = $1`, msgID)
	testPool.Exec(ctx, `DELETE FROM channels WHERE id = $1`, channelID)
	testPool.Exec(ctx, `DELETE FROM guilds WHERE id = $1`, guildID)
	testPool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
}

// --- NATS Event Bus Integration Tests ---

func TestEventBusHealthCheck(t *testing.T) {
	if err := testBus.HealthCheck(); err != nil {
		t.Fatalf("NATS health check failed: %v", err)
	}
}

func TestEventBusPubSub(t *testing.T) {
	received := make(chan events.Event, 1)

	_, err := testBus.Subscribe("amityvox.test.integration", func(event events.Event) {
		received <- event
	})
	if err != nil {
		t.Fatalf("subscribing: %v", err)
	}

	// Give subscription time to establish.
	time.Sleep(100 * time.Millisecond)

	// Publish event.
	data, _ := json.Marshal(map[string]string{"key": "value"})
	err = testBus.Publish(context.Background(), "amityvox.test.integration", events.Event{
		Type: "TEST_EVENT",
		Data: data,
	})
	if err != nil {
		t.Fatalf("publishing: %v", err)
	}

	// Wait for event.
	select {
	case event := <-received:
		if event.Type != "TEST_EVENT" {
			t.Errorf("expected event type TEST_EVENT, got %s", event.Type)
		}
		var payload map[string]string
		json.Unmarshal(event.Data, &payload)
		if payload["key"] != "value" {
			t.Errorf("expected key=value in payload, got %v", payload)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestEventBusQueueSubscribe(t *testing.T) {
	count := make(chan struct{}, 10)

	// Two queue subscribers — only one should receive each message.
	for i := 0; i < 2; i++ {
		testBus.QueueSubscribe("amityvox.test.queue", "test-group", func(event events.Event) {
			count <- struct{}{}
		})
	}

	time.Sleep(100 * time.Millisecond)

	// Publish 3 messages.
	for i := 0; i < 3; i++ {
		data, _ := json.Marshal(map[string]int{"n": i})
		testBus.Publish(context.Background(), "amityvox.test.queue", events.Event{
			Type: "TEST_QUEUE",
			Data: data,
		})
	}

	// Should receive exactly 3 (one per message, not duplicated).
	received := 0
	timeout := time.After(5 * time.Second)
	for received < 3 {
		select {
		case <-count:
			received++
		case <-timeout:
			t.Fatalf("timed out: only received %d/3 messages", received)
		}
	}

	// Brief wait to ensure no extras arrive.
	time.Sleep(200 * time.Millisecond)
	if len(count) > 0 {
		t.Errorf("received extra messages beyond expected 3")
	}
}

// --- Cache Integration Tests ---

func TestCacheHealthCheck(t *testing.T) {
	if err := testCache.HealthCheck(context.Background()); err != nil {
		t.Fatalf("cache health check failed: %v", err)
	}
}

func TestCacheSetGet(t *testing.T) {
	ctx := context.Background()
	key := "integration_test_" + models.NewULID().String()

	err := testCache.Set(ctx, key, "hello-world", 30*time.Second)
	if err != nil {
		t.Fatalf("cache set: %v", err)
	}

	var val string
	found, err := testCache.Get(ctx, key, &val)
	if err != nil {
		t.Fatalf("cache get: %v", err)
	}
	if !found {
		t.Fatal("expected key to be found")
	}
	if val != "hello-world" {
		t.Errorf("expected 'hello-world', got %q", val)
	}

	// Delete.
	testCache.Delete(ctx, key)
	found, err = testCache.Get(ctx, key, &val)
	if err != nil {
		t.Fatalf("cache get after delete: %v", err)
	}
	if found {
		t.Error("expected key not to be found after delete")
	}
}

// --- Auth Service Integration Test ---

func TestAuthRegisterAndLogin(t *testing.T) {
	ctx := context.Background()

	// Ensure instance exists.
	var instanceID string
	testPool.QueryRow(ctx, `SELECT id FROM instances LIMIT 1`).Scan(&instanceID)
	if instanceID == "" {
		instanceID = models.NewULID().String()
		testPool.Exec(ctx,
			`INSERT INTO instances (id, domain, public_key, name, software_version, federation_mode, created_at)
			 VALUES ($1, 'test.local', 'test-key', 'Test', 'test', 'closed', now())`,
			instanceID)
	}

	authSvc := auth.NewService(auth.Config{
		Pool:            testPool,
		Cache:           testCache,
		InstanceID:      instanceID,
		SessionDuration: 24 * time.Hour,
		RegEnabled:      true,
		InviteOnly:      false,
		RequireEmail:    false,
		Logger:          testLogger,
	})

	username := "authtest_" + models.NewULID().String()[:8]
	password := "Test1234!Secure"

	// Register.
	user, session, err := authSvc.Register(ctx, auth.RegisterRequest{
		Username: username,
		Password: password,
	}, "127.0.0.1", "integration-test")
	if err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	if user.Username != username {
		t.Errorf("expected username %q, got %q", username, user.Username)
	}
	if session.ID == "" {
		t.Error("expected session ID to be set")
	}

	// Login.
	user2, session2, err := authSvc.Login(ctx, auth.LoginRequest{
		Username: username,
		Password: password,
	}, "127.0.0.1", "integration-test")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	if user2.ID != user.ID {
		t.Error("login returned different user ID")
	}
	if session2.ID == session.ID {
		t.Error("login should create a new session")
	}

	// Clean up.
	testPool.Exec(ctx, `DELETE FROM user_sessions WHERE user_id = $1`, user.ID)
	testPool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user.ID)
}

// --- HTTP Handler Integration Test ---

func TestHealthEndpoint(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := map[string]string{"status": "ok"}

		if err := testDB.HealthCheck(r.Context()); err != nil {
			status["database"] = "unhealthy"
		} else {
			status["database"] = "healthy"
		}

		if err := testBus.HealthCheck(); err != nil {
			status["nats"] = "unhealthy"
		} else {
			status["nats"] = "healthy"
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %q", body["status"])
	}
	if body["database"] != "healthy" {
		t.Errorf("expected database healthy, got %q", body["database"])
	}
	if body["nats"] != "healthy" {
		t.Errorf("expected nats healthy, got %q", body["nats"])
	}
}

// --- Migration Integrity Test ---

func TestMigrationTables(t *testing.T) {
	ctx := context.Background()

	expectedTables := []string{
		"users", "guilds", "channels", "messages", "guild_members",
		"roles", "member_roles", "channel_overrides", "user_sessions",
		"invites", "guild_bans", "attachments", "reactions",
		"read_state", "instances", "federation_peers", "custom_emoji",
		"webhooks", "audit_log",
		"mls_key_packages", "mls_welcome_messages", "mls_group_states", "mls_commits",
		"automod_rules", "automod_actions",
		"push_subscriptions", "notification_preferences",
	}

	for _, table := range expectedTables {
		var exists bool
		err := testPool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1)`,
			table).Scan(&exists)
		if err != nil {
			t.Errorf("checking table %s: %v", table, err)
			continue
		}
		if !exists {
			t.Errorf("expected table %q to exist", table)
		}
	}
}

// TestAutomodRuleCRUD tests the automod rule lifecycle.
func TestAutomodRuleCRUD(t *testing.T) {
	ctx := context.Background()

	// Create test guild + user.
	var instanceID string
	testPool.QueryRow(ctx, `SELECT id FROM instances LIMIT 1`).Scan(&instanceID)

	userID := models.NewULID().String()
	testPool.Exec(ctx,
		`INSERT INTO users (id, instance_id, username, password_hash, created_at)
		 VALUES ($1, $2, $3, 'hash', now())`,
		userID, instanceID, "automod_test_"+userID[:6])

	guildID := models.NewULID().String()
	testPool.Exec(ctx,
		`INSERT INTO guilds (id, name, owner_id, created_at) VALUES ($1, $2, $3, now())`,
		guildID, "AutoMod Test Guild", userID)

	// Create rule.
	ruleID := models.NewULID().String()
	config := `{"words": ["badword"], "match_whole_word": true}`
	_, err := testPool.Exec(ctx,
		`INSERT INTO automod_rules (id, guild_id, name, enabled, rule_type, config, action, created_by, created_at, updated_at)
		 VALUES ($1, $2, $3, true, 'word_filter', $4, 'delete', $5, now(), now())`,
		ruleID, guildID, "Bad Word Filter", config, userID)
	if err != nil {
		t.Fatalf("creating automod rule: %v", err)
	}

	// Query rule.
	var name, ruleType string
	err = testPool.QueryRow(ctx,
		`SELECT name, rule_type FROM automod_rules WHERE id = $1`, ruleID).Scan(&name, &ruleType)
	if err != nil {
		t.Fatalf("querying rule: %v", err)
	}
	if name != "Bad Word Filter" || ruleType != "word_filter" {
		t.Errorf("unexpected rule: name=%q type=%q", name, ruleType)
	}

	// Update rule.
	_, err = testPool.Exec(ctx,
		`UPDATE automod_rules SET enabled = false, updated_at = now() WHERE id = $1`, ruleID)
	if err != nil {
		t.Fatalf("updating rule: %v", err)
	}

	var enabled bool
	testPool.QueryRow(ctx, `SELECT enabled FROM automod_rules WHERE id = $1`, ruleID).Scan(&enabled)
	if enabled {
		t.Error("expected rule to be disabled")
	}

	// Delete rule.
	_, err = testPool.Exec(ctx, `DELETE FROM automod_rules WHERE id = $1`, ruleID)
	if err != nil {
		t.Fatalf("deleting rule: %v", err)
	}

	// Clean up.
	testPool.Exec(ctx, `DELETE FROM guilds WHERE id = $1`, guildID)
	testPool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
}

// helperParseJSON parses a JSON response body.
func helperParseJSON(body string) map[string]interface{} {
	var result map[string]interface{}
	json.NewDecoder(strings.NewReader(body)).Decode(&result)
	return result
}
