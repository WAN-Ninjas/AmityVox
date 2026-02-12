// Package scanning provides file content scanning integration for AmityVox.
// It supports ClamAV virus scanning via the clamd TCP protocol, used to scan
// uploaded files before they are stored in S3.
package scanning

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"
)

// ClamAVConfig holds ClamAV daemon connection and scanning configuration.
type ClamAVConfig struct {
	// Enabled controls whether file scanning is active. When false, all files
	// are allowed through without scanning.
	Enabled bool `toml:"enabled"`

	// Address is the clamd TCP address (e.g., "localhost:3310").
	Address string `toml:"address"`

	// Timeout is the maximum time to wait for a scan result.
	Timeout time.Duration `toml:"timeout"`

	// MaxFileSize is the maximum file size to scan in bytes. Files larger than
	// this are rejected without scanning. Defaults to 100MB.
	MaxFileSize int64 `toml:"max_file_size"`

	// PoolSize is the number of persistent connections to maintain to clamd.
	PoolSize int `toml:"pool_size"`
}

// DefaultClamAVConfig returns sensible defaults for ClamAV integration.
func DefaultClamAVConfig() ClamAVConfig {
	return ClamAVConfig{
		Enabled:     false,
		Address:     "localhost:3310",
		Timeout:     30 * time.Second,
		MaxFileSize: 100 * 1024 * 1024, // 100MB
		PoolSize:    3,
	}
}

// ScanResult represents the outcome of a file scan.
type ScanResult struct {
	// Clean is true if no threats were detected.
	Clean bool

	// Threat is the name of the detected threat, if any.
	Threat string

	// Duration is how long the scan took.
	Duration time.Duration

	// FileSize is the size of the scanned file in bytes.
	FileSize int64
}

// Scanner provides an interface for file content scanning. This abstraction
// allows swapping ClamAV for other scanning backends in the future.
type Scanner interface {
	// Scan checks the given reader for malicious content. Returns a ScanResult
	// indicating whether the content is clean or contains threats.
	Scan(ctx context.Context, reader io.Reader, filename string, size int64) (*ScanResult, error)

	// HealthCheck verifies that the scanning service is available and responsive.
	HealthCheck(ctx context.Context) error

	// Close releases all resources held by the scanner.
	Close() error
}

// ClamAVScanner implements the Scanner interface using the ClamAV daemon (clamd)
// via its TCP protocol. It maintains a connection pool for efficient scanning.
type ClamAVScanner struct {
	config ClamAVConfig
	pool   chan net.Conn
	logger *slog.Logger
	mu     sync.Mutex
	closed bool
}

// NewClamAVScanner creates a new ClamAV scanner with the given configuration.
// It pre-creates a pool of TCP connections to the clamd daemon.
func NewClamAVScanner(cfg ClamAVConfig, logger *slog.Logger) (*ClamAVScanner, error) {
	if !cfg.Enabled {
		return &ClamAVScanner{config: cfg, logger: logger}, nil
	}

	scanner := &ClamAVScanner{
		config: cfg,
		pool:   make(chan net.Conn, cfg.PoolSize),
		logger: logger,
	}

	// Pre-fill the connection pool.
	for i := 0; i < cfg.PoolSize; i++ {
		conn, err := scanner.connect()
		if err != nil {
			// Close any already-created connections.
			scanner.Close()
			return nil, fmt.Errorf("creating ClamAV connection pool: %w", err)
		}
		scanner.pool <- conn
	}

	logger.Info("ClamAV scanner initialized",
		slog.String("address", cfg.Address),
		slog.Int("pool_size", cfg.PoolSize),
	)

	return scanner, nil
}

// Scan checks the given file content for malicious payloads using ClamAV's
// INSTREAM command. The file is streamed to clamd in chunks without buffering
// the entire file in memory.
func (s *ClamAVScanner) Scan(ctx context.Context, reader io.Reader, filename string, size int64) (*ScanResult, error) {
	if !s.config.Enabled {
		return &ScanResult{Clean: true, FileSize: size}, nil
	}

	if size > s.config.MaxFileSize {
		return nil, fmt.Errorf("file size %d exceeds maximum scannable size %d", size, s.config.MaxFileSize)
	}

	start := time.Now()

	conn, err := s.getConn(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting ClamAV connection: %w", err)
	}
	defer s.returnConn(conn)

	// Set deadline for the entire scan operation.
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(s.config.Timeout)
	}
	conn.SetDeadline(deadline)

	// Send INSTREAM command.
	if _, err := conn.Write([]byte("zINSTREAM\x00")); err != nil {
		return nil, fmt.Errorf("sending INSTREAM command: %w", err)
	}

	// Stream file data in chunks. ClamAV expects each chunk prefixed with its
	// length as a 4-byte big-endian integer, followed by a zero-length chunk
	// to signal end of stream.
	buf := make([]byte, 32768) // 32KB chunks
	totalRead := int64(0)

	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			totalRead += int64(n)

			// Write chunk length (4 bytes, big-endian).
			chunkLen := []byte{
				byte(n >> 24),
				byte(n >> 16),
				byte(n >> 8),
				byte(n),
			}
			if _, err := conn.Write(chunkLen); err != nil {
				return nil, fmt.Errorf("writing chunk length: %w", err)
			}

			// Write chunk data.
			if _, err := conn.Write(buf[:n]); err != nil {
				return nil, fmt.Errorf("writing chunk data: %w", err)
			}
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("reading file data: %w", readErr)
		}
	}

	// Send zero-length chunk to signal end of stream.
	if _, err := conn.Write([]byte{0, 0, 0, 0}); err != nil {
		return nil, fmt.Errorf("sending end-of-stream marker: %w", err)
	}

	// Read response.
	response := make([]byte, 4096)
	n, err := conn.Read(response)
	if err != nil {
		return nil, fmt.Errorf("reading ClamAV response: %w", err)
	}

	result := strings.TrimSpace(string(response[:n]))
	// Remove null terminators from zINSTREAM response.
	result = strings.TrimRight(result, "\x00")
	duration := time.Since(start)

	s.logger.Debug("ClamAV scan completed",
		slog.String("filename", filename),
		slog.Int64("size", totalRead),
		slog.Duration("duration", duration),
		slog.String("result", result),
	)

	scanResult := &ScanResult{
		Duration: duration,
		FileSize: totalRead,
	}

	if strings.HasSuffix(result, "OK") {
		scanResult.Clean = true
	} else if strings.Contains(result, "FOUND") {
		scanResult.Clean = false
		// Extract threat name: "stream: ThreatName FOUND"
		parts := strings.SplitN(result, ":", 2)
		if len(parts) == 2 {
			threat := strings.TrimSpace(parts[1])
			threat = strings.TrimSuffix(threat, " FOUND")
			scanResult.Threat = threat
		}
		s.logger.Warn("ClamAV detected threat",
			slog.String("filename", filename),
			slog.String("threat", scanResult.Threat),
		)
	} else if strings.Contains(result, "ERROR") {
		return nil, fmt.Errorf("ClamAV scan error: %s", result)
	}

	return scanResult, nil
}

// HealthCheck verifies the ClamAV daemon is responsive by sending a PING command.
func (s *ClamAVScanner) HealthCheck(ctx context.Context) error {
	if !s.config.Enabled {
		return nil
	}

	conn, err := s.getConn(ctx)
	if err != nil {
		return fmt.Errorf("ClamAV health check connection: %w", err)
	}
	defer s.returnConn(conn)

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Send PING command.
	if _, err := conn.Write([]byte("zPING\x00")); err != nil {
		return fmt.Errorf("ClamAV PING failed: %w", err)
	}

	// Read PONG response.
	response := make([]byte, 64)
	n, err := conn.Read(response)
	if err != nil {
		return fmt.Errorf("ClamAV PONG read failed: %w", err)
	}

	result := strings.TrimSpace(strings.TrimRight(string(response[:n]), "\x00"))
	if result != "PONG" {
		return fmt.Errorf("ClamAV unexpected response to PING: %q", result)
	}

	return nil
}

// Close releases all connections in the pool and marks the scanner as closed.
func (s *ClamAVScanner) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	if s.pool == nil {
		return nil
	}

	close(s.pool)
	for conn := range s.pool {
		if conn != nil {
			conn.Close()
		}
	}

	s.logger.Info("ClamAV scanner closed")
	return nil
}

// connect creates a new TCP connection to the clamd daemon.
func (s *ClamAVScanner) connect() (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", s.config.Address, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connecting to ClamAV at %s: %w", s.config.Address, err)
	}
	return conn, nil
}

// getConn retrieves a connection from the pool or creates a new one.
func (s *ClamAVScanner) getConn(ctx context.Context) (net.Conn, error) {
	select {
	case conn := <-s.pool:
		if conn != nil {
			// Test the connection is still alive.
			conn.SetDeadline(time.Now().Add(time.Second))
			one := make([]byte, 1)
			conn.SetReadDeadline(time.Now().Add(time.Millisecond))
			_, err := conn.Read(one)
			if err != nil && !isTimeout(err) {
				// Connection is dead, create a new one.
				conn.Close()
				return s.connect()
			}
			conn.SetDeadline(time.Time{})
			return conn, nil
		}
		return s.connect()
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return s.connect()
	}
}

// returnConn returns a connection to the pool. If the pool is full, the
// connection is closed.
func (s *ClamAVScanner) returnConn(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || conn == nil {
		if conn != nil {
			conn.Close()
		}
		return
	}

	select {
	case s.pool <- conn:
		// Returned to pool.
	default:
		// Pool full, close the connection.
		conn.Close()
	}
}

// isTimeout checks whether an error is a network timeout.
func isTimeout(err error) bool {
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	return false
}

// NoOpScanner is a Scanner implementation that always returns clean results.
// Used when ClamAV scanning is disabled.
type NoOpScanner struct{}

// Scan always returns a clean result.
func (s *NoOpScanner) Scan(_ context.Context, _ io.Reader, _ string, size int64) (*ScanResult, error) {
	return &ScanResult{Clean: true, FileSize: size}, nil
}

// HealthCheck always returns nil.
func (s *NoOpScanner) HealthCheck(_ context.Context) error {
	return nil
}

// Close is a no-op.
func (s *NoOpScanner) Close() error {
	return nil
}
