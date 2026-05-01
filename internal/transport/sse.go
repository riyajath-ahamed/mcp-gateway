package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// SSETransport connects to an MCP server that uses Server-Sent Events for streaming
// responses. It sends requests as HTTP POST and reads back SSE data frames.
type SSETransport struct {
	url     string
	headers map[string]string
	client  *http.Client
	logger  *slog.Logger
	nextID  atomic.Int64
	mu      sync.Mutex
}

// NewSSE creates a new SSE-based MCP transport.
func NewSSE(url string, headers map[string]string, logger *slog.Logger) *SSETransport {
	return &SSETransport{
		url:     url,
		headers: headers,
		client:  &http.Client{Timeout: 0}, // no timeout — SSE is long-lived
		logger:  logger,
	}
}

// Call sends a JSON-RPC request and collects the full SSE response.
// For streaming consumers, use CallStream instead.
func (t *SSETransport) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	var result json.RawMessage
	err := t.CallStream(ctx, method, params, func(data []byte) error {
		// Last data frame wins (some servers send partial progress frames first)
		result = json.RawMessage(data)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("no data received from SSE stream")
	}
	return result, nil
}

// CallStream sends a JSON-RPC request and calls onData for each SSE data frame.
func (t *SSETransport) CallStream(ctx context.Context, method string, params any, onData func([]byte) error) error {
	id := t.nextID.Add(1)
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url+"/mcp", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		// Fallback: treat as plain JSON response
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("reading response: %w", err)
		}
		return onData(data)
	}

	return t.readSSE(resp.Body, onData)
}

// readSSE reads newline-delimited SSE frames from r and calls onData for each data: line.
func (t *SSETransport) readSSE(r io.Reader, onData func([]byte) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	var eventBuf strings.Builder
	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case line == "":
			// Empty line = end of event
			if eventBuf.Len() > 0 {
				if err := onData([]byte(eventBuf.String())); err != nil {
					return err
				}
				eventBuf.Reset()
			}
		case strings.HasPrefix(line, "data: "):
			data := strings.TrimPrefix(line, "data: ")
			// Check for SSE terminal sentinel
			if data == "[DONE]" {
				return nil
			}
			if eventBuf.Len() > 0 {
				eventBuf.WriteByte('\n')
			}
			eventBuf.WriteString(data)
		case strings.HasPrefix(line, "event: "):
			// Named events (e.g. "event: error") — log and continue
			t.logger.Debug("SSE event", "type", strings.TrimPrefix(line, "event: "))
		case strings.HasPrefix(line, ": "):
			// SSE comment / heartbeat — ignore
		}
	}

	// Flush any trailing data
	if eventBuf.Len() > 0 {
		return onData([]byte(eventBuf.String()))
	}

	return scanner.Err()
}

// Ping sends a lightweight ping to verify the SSE server is alive.
func (t *SSETransport) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := t.Call(ctx, "ping", map[string]any{})
	return err
}
