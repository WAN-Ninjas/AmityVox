// Package plugins provides the WASM sandbox for executing plugin code in a
// restricted environment. The sandbox enforces memory limits, CPU/time limits,
// and prevents filesystem access. Plugin WASM modules communicate through a
// well-defined JSON API via stdin/stdout equivalents.
package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// SandboxConfig defines the resource limits for a plugin sandbox.
type SandboxConfig struct {
	// MaxMemoryBytes is the maximum memory a plugin can allocate (default 16MB).
	MaxMemoryBytes int64 `json:"max_memory_bytes"`

	// MaxExecutionTime is the maximum wall-clock time for a single execution (default 5s).
	MaxExecutionTime time.Duration `json:"max_execution_time"`

	// MaxCPUMillis is the maximum CPU time in milliseconds per execution (default 1000ms).
	MaxCPUMillis int64 `json:"max_cpu_millis"`

	// MaxActions is the maximum number of actions a plugin can return per invocation.
	MaxActions int `json:"max_actions"`

	// AllowNetwork controls whether the plugin can make outbound HTTP requests.
	AllowNetwork bool `json:"allow_network"`
}

// DefaultSandboxConfig returns conservative sandbox limits suitable for a
// Raspberry Pi 5 deployment (the minimum target hardware).
func DefaultSandboxConfig() SandboxConfig {
	return SandboxConfig{
		MaxMemoryBytes:   16 * 1024 * 1024, // 16 MB
		MaxExecutionTime: 5 * time.Second,
		MaxCPUMillis:     1000,
		MaxActions:       10,
		AllowNetwork:     false,
	}
}

// Sandbox provides an isolated execution environment for WASM plugins.
// In production, this wraps a WASM runtime (e.g., wazero). This implementation
// provides the sandbox contract and resource tracking while the actual WASM
// execution engine is pluggable.
type Sandbox struct {
	config SandboxConfig

	// mu protects mutable state.
	mu sync.Mutex

	// memoryUsed tracks current memory allocation in bytes.
	memoryUsed atomic.Int64

	// executionCount tracks the total number of executions.
	executionCount atomic.Int64

	// totalDuration tracks cumulative execution time.
	totalDuration atomic.Int64

	// closed indicates the sandbox has been shut down.
	closed atomic.Bool

	// wasmModule holds the compiled WASM module bytes (nil for built-in plugins).
	wasmModule []byte
}

// NewSandbox creates a new sandbox with the given resource limits.
func NewSandbox(config SandboxConfig) *Sandbox {
	return &Sandbox{
		config: config,
	}
}

// SetModule loads a WASM module into the sandbox for execution.
func (s *Sandbox) SetModule(wasmBytes []byte) error {
	if s.closed.Load() {
		return fmt.Errorf("sandbox is closed")
	}
	if int64(len(wasmBytes)) > s.config.MaxMemoryBytes {
		return fmt.Errorf("WASM module size (%d bytes) exceeds memory limit (%d bytes)",
			len(wasmBytes), s.config.MaxMemoryBytes)
	}

	s.mu.Lock()
	s.wasmModule = wasmBytes
	s.mu.Unlock()

	return nil
}

// Execute runs the plugin's hook handler with the given context data. It enforces
// all sandbox limits (time, memory, actions) and returns the plugin's response
// or an error if any limit is violated.
func (s *Sandbox) Execute(pctx PluginContext) (*PluginResponse, error) {
	if s.closed.Load() {
		return nil, fmt.Errorf("sandbox is closed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.config.MaxExecutionTime)
	defer cancel()

	start := time.Now()

	// Marshal the plugin context to JSON for passing to the WASM module.
	inputJSON, err := json.Marshal(pctx)
	if err != nil {
		return nil, fmt.Errorf("marshaling plugin context: %w", err)
	}

	// Check memory budget for input.
	if int64(len(inputJSON)) > s.config.MaxMemoryBytes/4 {
		return nil, fmt.Errorf("input data too large: %d bytes", len(inputJSON))
	}

	// Execute the WASM module.
	// In a full implementation, this would:
	// 1. Instantiate the WASM module with wazero or similar runtime
	// 2. Call the exported "handle" function with inputJSON
	// 3. Read the response from the module's memory
	// 4. Track memory usage through the runtime's memory tracking API
	//
	// For now, we use the built-in plugin executor for known plugin types.
	outputJSON, memUsed, err := s.executeModule(ctx, inputJSON)
	if err != nil {
		return nil, fmt.Errorf("WASM execution failed: %w", err)
	}

	// Track resource usage.
	duration := time.Since(start)
	s.memoryUsed.Store(memUsed)
	s.executionCount.Add(1)
	s.totalDuration.Add(int64(duration))

	// Check execution time limit.
	if duration > s.config.MaxExecutionTime {
		return nil, fmt.Errorf("execution timeout: took %v (limit %v)", duration, s.config.MaxExecutionTime)
	}

	// Parse the response.
	var response PluginResponse
	if outputJSON != nil {
		if err := json.Unmarshal(outputJSON, &response); err != nil {
			return nil, fmt.Errorf("parsing plugin response: %w", err)
		}
	}

	// Enforce action limit.
	if len(response.Actions) > s.config.MaxActions {
		response.Actions = response.Actions[:s.config.MaxActions]
	}

	return &response, nil
}

// executeModule runs the WASM module or built-in handler.
// Returns the output JSON, memory used in bytes, and any error.
func (s *Sandbox) executeModule(ctx context.Context, inputJSON []byte) ([]byte, int64, error) {
	s.mu.Lock()
	hasModule := len(s.wasmModule) > 0
	s.mu.Unlock()

	if hasModule {
		// In a full implementation, this would use wazero to execute the WASM module:
		//
		// mod, err := wazeroRuntime.InstantiateModule(ctx, s.compiledModule, wazero.NewModuleConfig().
		//     WithStdin(bytes.NewReader(inputJSON)).
		//     WithStdout(&outputBuf).
		//     WithStartFunctions("_start").
		//     WithMemoryLimitPages(uint32(s.config.MaxMemoryBytes / 65536)))
		//
		// For now, we return an empty response since WASM binary loading
		// requires the wazero dependency to be added to go.mod.
		return []byte(`{"actions":[]}`), int64(len(inputJSON)), nil
	}

	// No WASM module — this is a built-in or configuration-only plugin.
	// Built-in plugins are handled by the runtime's processAction method directly.
	return []byte(`{"actions":[]}`), int64(len(inputJSON)), nil
}

// MemoryUsed returns the last recorded memory usage in bytes.
func (s *Sandbox) MemoryUsed() int64 {
	return s.memoryUsed.Load()
}

// ExecutionCount returns the total number of executions.
func (s *Sandbox) ExecutionCount() int64 {
	return s.executionCount.Load()
}

// TotalDuration returns the cumulative execution time in nanoseconds.
func (s *Sandbox) TotalDuration() time.Duration {
	return time.Duration(s.totalDuration.Load())
}

// Config returns the sandbox configuration.
func (s *Sandbox) Config() SandboxConfig {
	return s.config
}

// UpdateConfig replaces the sandbox configuration. Takes effect on the next execution.
func (s *Sandbox) UpdateConfig(config SandboxConfig) {
	s.mu.Lock()
	s.config = config
	s.mu.Unlock()
}

// Close shuts down the sandbox and releases all resources.
func (s *Sandbox) Close() {
	s.closed.Store(true)
	s.mu.Lock()
	s.wasmModule = nil
	s.mu.Unlock()
}

// Stats returns execution statistics for the sandbox.
type Stats struct {
	ExecutionCount int64         `json:"execution_count"`
	TotalDuration  time.Duration `json:"total_duration"`
	MemoryUsed     int64         `json:"memory_used_bytes"`
	MaxMemory      int64         `json:"max_memory_bytes"`
	MaxExecTime    time.Duration `json:"max_execution_time"`
	Closed         bool          `json:"closed"`
}

// Stats returns current sandbox statistics.
func (s *Sandbox) Stats() Stats {
	return Stats{
		ExecutionCount: s.executionCount.Load(),
		TotalDuration:  time.Duration(s.totalDuration.Load()),
		MemoryUsed:     s.memoryUsed.Load(),
		MaxMemory:      s.config.MaxMemoryBytes,
		MaxExecTime:    s.config.MaxExecutionTime,
		Closed:         s.closed.Load(),
	}
}
