// Package tunnel connects the CLI to the Sibil Monitor WebSocket relay.
//
// Privacy model: the backend routes opaque bytes between this agent and
// the mobile client. It never parses or stores infrastructure data.
// The CLI token is the shared secret — only the mobile that knows the
// token can connect to this agent via the relay.
package tunnel

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type tunnelRequest struct {
	ID      string            `json:"id"`
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type tunnelResponse struct {
	ID     string `json:"id"`
	Status int    `json:"status"`
	Body   string `json:"body"`
}

const (
	backoffBase = 2 * time.Second
	backoffMax  = 5 * time.Minute
)

// Run connects to the backend WebSocket relay and proxies incoming requests
// to the local CLI HTTP server. It reconnects automatically on disconnect
// using exponential backoff (2s → 4s → … → 5min).
func Run(backendWS, cliToken string, localPort int) {
	delay := backoffBase
	for {
		err := connect(backendWS, cliToken, localPort)
		fmt.Printf("[tunnel] disconnected: %v — reconnecting in %s\n", err, delay)
		time.Sleep(delay)
		delay *= 2
		if delay > backoffMax {
			delay = backoffMax
		}
		// Reset backoff on a successful connection that lasted long enough
		// (connect() returns only on error, so any return is a disconnect).
	}
}

func connect(backendWS, cliToken string, localPort int) error {
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, _, err := dialer.Dial(backendWS, nil)
	if err != nil {
		return fmt.Errorf("dial %s: %w", backendWS, err)
	}
	defer conn.Close()

	// Register as agent.
	if err := conn.WriteJSON(map[string]string{"type": "register", "token": cliToken}); err != nil {
		return fmt.Errorf("register write: %w", err)
	}

	var ack map[string]string
	if err := conn.ReadJSON(&ack); err != nil {
		return fmt.Errorf("register ack: %w", err)
	}
	if ack["type"] != "registered" {
		return fmt.Errorf("registration rejected: %v", ack["message"])
	}
	agentID := ack["agent_id"]
	fmt.Printf("[tunnel] connected — agent %s…\n", agentID[:12])

	client := &http.Client{Timeout: 29 * time.Second}
	var writeMu sync.Mutex

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		var req tunnelRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			continue
		}

		// Proxy each request in its own goroutine so slow requests don't block others.
		go func(req tunnelRequest) {
			resp := proxyLocal(client, req, localPort)
			writeMu.Lock()
			defer writeMu.Unlock()
			_ = conn.WriteJSON(resp)
		}(req)
	}
}

// proxyLocal forwards a tunnelled request to the CLI's own local HTTP server.
// Authentication is enforced by the existing bearerAuth middleware.
func proxyLocal(client *http.Client, req tunnelRequest, port int) tunnelResponse {
	target := fmt.Sprintf("http://127.0.0.1:%d%s", port, req.Path)

	var bodyReader io.Reader = http.NoBody
	if req.Body != "" {
		bodyReader = strings.NewReader(req.Body)
	}

	httpReq, err := http.NewRequest(req.Method, target, bodyReader)
	if err != nil {
		return tunnelResponse{ID: req.ID, Status: 400, Body: `{"error":"bad request"}`}
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return tunnelResponse{ID: req.ID, Status: 503, Body: `{"error":"local CLI unavailable"}`}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return tunnelResponse{ID: req.ID, Status: resp.StatusCode, Body: string(body)}
}
