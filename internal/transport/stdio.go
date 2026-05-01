package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
)

// StdioTransport manages a single subprocess MCP server,
// communicating via newline-delimited JSON-RPC over stdin/stdout.
type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	logger *slog.Logger

	mu      sync.Mutex
	nextID  atomic.Int64
	pending map[int64]chan json.RawMessage
}

type jsonRPC struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewStdio starts the subprocess and returns a transport ready to use.
func NewStdio(ctx context.Context, command string, env map[string]string, logger *slog.Logger) (*StdioTransport, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)

	// Inject env vars
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting subprocess: %w", err)
	}

	t := &StdioTransport{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewReader(stdoutPipe),
		logger:  logger,
		pending: make(map[int64]chan json.RawMessage),
	}

	// MCP initialization handshake
	if err := t.initialize(ctx); err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("mcp init: %w", err)
	}

	// Start response demultiplexer
	go t.readLoop()

	logger.Info("stdio server started", "command", command, "pid", cmd.Process.Pid)
	return t, nil
}

func (t *StdioTransport) initialize(ctx context.Context) error {
	initParams := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "mcp-gateway",
			"version": "0.1.0",
		},
	}
	_, err := t.Call(ctx, "initialize", initParams)
	if err != nil {
		return err
	}
	// Send initialized notification
	notif := jsonRPC{JSONRPC: "2.0", Method: "notifications/initialized"}
	return t.send(notif)
}

// Call sends a JSON-RPC request and waits for the response.
func (t *StdioTransport) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := t.nextID.Add(1)

	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshaling params: %w", err)
	}

	ch := make(chan json.RawMessage, 1)
	t.mu.Lock()
	t.pending[id] = ch
	t.mu.Unlock()

	req := jsonRPC{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  paramsRaw,
	}
	if err := t.send(req); err != nil {
		t.mu.Lock()
		delete(t.pending, id)
		t.mu.Unlock()
		return nil, err
	}

	select {
	case <-ctx.Done():
		t.mu.Lock()
		delete(t.pending, id)
		t.mu.Unlock()
		return nil, ctx.Err()
	case result := <-ch:
		return result, nil
	}
}

// ListTools calls tools/list and returns the raw result JSON.
func (t *StdioTransport) ListTools(ctx context.Context) (json.RawMessage, error) {
	return t.Call(ctx, "tools/list", map[string]any{})
}

// CallTool calls tools/call with the given tool name and arguments.
func (t *StdioTransport) CallTool(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	return t.Call(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
}

func (t *StdioTransport) send(msg jsonRPC) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	t.mu.Lock()
	defer t.mu.Unlock()
	_, err = t.stdin.Write(data)
	return err
}

// readLoop reads newline-delimited JSON-RPC messages and dispatches responses
// to the waiting callers via the pending map.
func (t *StdioTransport) readLoop() {
	for {
		line, err := t.stdout.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				t.logger.Error("stdio read error", "error", err)
			}
			return
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		var msg jsonRPC
		if err := json.Unmarshal(line, &msg); err != nil {
			t.logger.Warn("unparseable message from subprocess", "raw", string(line))
			continue
		}

		// Notifications have no ID — log and discard
		if msg.ID == 0 && msg.Method != "" {
			t.logger.Debug("notification from subprocess", "method", msg.Method)
			continue
		}

		t.mu.Lock()
		ch, ok := t.pending[msg.ID]
		if ok {
			delete(t.pending, msg.ID)
		}
		t.mu.Unlock()

		if !ok {
			t.logger.Warn("response for unknown ID", "id", msg.ID)
			continue
		}

		if msg.Error != nil {
			// Pack error into result channel as a JSON error object
			errJSON, _ := json.Marshal(msg.Error)
			ch <- errJSON
		} else {
			ch <- msg.Result
		}
	}
}

// Close terminates the subprocess.
func (t *StdioTransport) Close() error {
	t.stdin.Close()
	return t.cmd.Wait()
}
