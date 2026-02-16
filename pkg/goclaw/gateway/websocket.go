// Package gateway – websocket.go implements a bidirectional JSON-RPC WebSocket
// endpoint for real-time communication with the GoClaw agent.
//
// Protocol:
//
//	Client → Server (requests):
//	  {"type":"req","id":"1","method":"chat.send","params":{"sessionId":"...","content":"..."}}
//	  {"type":"req","id":"2","method":"chat.abort","params":{"sessionId":"..."}}
//	  {"type":"req","id":"3","method":"chat.history","params":{"sessionId":"..."}}
//
//	Server → Client (responses):
//	  {"type":"res","id":"1","ok":true,"payload":{"runId":"..."}}
//
//	Server → Client (events — unsolicited):
//	  {"type":"event","event":"delta","payload":{"content":"..."}}
//	  {"type":"event","event":"tool_use","payload":{"tool":"...","input":{...}}}
//	  {"type":"event","event":"done","payload":{"usage":{...}}}
package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/jholhewres/goclaw/pkg/goclaw/copilot"
	"github.com/jholhewres/goclaw/pkg/goclaw/webui"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// wsMessage is the envelope for all WebSocket messages.
type wsMessage struct {
	Type    string          `json:"type"`              // "req", "res", "event"
	ID      string          `json:"id,omitempty"`      // Request/response correlation ID
	Method  string          `json:"method,omitempty"`  // For requests: "chat.send", "chat.abort", etc.
	Params  json.RawMessage `json:"params,omitempty"`  // For requests
	OK      *bool           `json:"ok,omitempty"`      // For responses
	Payload json.RawMessage `json:"payload,omitempty"` // For responses and events
	Event   string          `json:"event,omitempty"`   // For events: "delta", "tool_use", etc.
	Error   string          `json:"error,omitempty"`   // For error responses
}

// WebSocketHandler upgrades HTTP connections to WebSocket and handles
// bidirectional JSON-RPC communication.
type WebSocketHandler struct {
	api    webui.AssistantAPI
	logger *slog.Logger
}

// NewWebSocketHandler creates a new WebSocket handler.
func NewWebSocketHandler(api webui.AssistantAPI, logger *slog.Logger) *WebSocketHandler {
	return &WebSocketHandler{
		api:    api,
		logger: logger.With("component", "websocket"),
	}
}

// ServeHTTP upgrades the connection and starts the message loop.
func (h *WebSocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("websocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	h.logger.Info("websocket client connected", "remote", r.RemoteAddr)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// writeMu protects concurrent writes to the WebSocket connection.
	var writeMu sync.Mutex

	sendMsg := func(msg wsMessage) {
		data, err := json.Marshal(msg)
		if err != nil {
			return
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.WriteMessage(websocket.TextMessage, data)
	}

	sendRes := func(id string, ok bool, payload any, errMsg string) {
		msg := wsMessage{Type: "res", ID: id}
		boolVal := ok
		msg.OK = &boolVal
		if errMsg != "" {
			msg.Error = errMsg
		}
		if payload != nil {
			data, _ := json.Marshal(payload)
			msg.Payload = data
		}
		sendMsg(msg)
	}

	sendEvent := func(event string, payload any) {
		msg := wsMessage{Type: "event", Event: event}
		data, _ := json.Marshal(payload)
		msg.Payload = data
		sendMsg(msg)
	}

	// Read loop — process incoming requests.
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				h.logger.Warn("websocket read error", "error", err)
			}
			return
		}

		var msg wsMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			sendRes("", false, nil, "invalid JSON")
			continue
		}

		if msg.Type != "req" {
			sendRes(msg.ID, false, nil, "expected type=req")
			continue
		}

		// Dispatch based on method.
		switch msg.Method {
		case "chat.send":
			go h.handleChatSend(ctx, msg, sendRes, sendEvent)
		case "chat.abort":
			go h.handleChatAbort(ctx, msg, sendRes)
		case "chat.history":
			go h.handleChatHistory(msg, sendRes)
		case "sessions.list":
			sessions := h.api.ListSessions()
			sendRes(msg.ID, true, sessions, "")
		default:
			sendRes(msg.ID, false, nil, "unknown method: "+msg.Method)
		}
	}
}

// handleChatSend processes a chat.send request, streams events via the WebSocket.
func (h *WebSocketHandler) handleChatSend(ctx context.Context, msg wsMessage, sendRes func(string, bool, any, string), sendEvent func(string, any)) {
	var params struct {
		SessionID string `json:"sessionId"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil || params.SessionID == "" || params.Content == "" {
		sendRes(msg.ID, false, nil, "missing sessionId or content")
		return
	}

	handle, err := h.api.StartChatStream(ctx, params.SessionID, params.Content)
	if err != nil {
		sendRes(msg.ID, false, nil, err.Error())
		return
	}

	// Send the run_id back as the response.
	sendRes(msg.ID, true, map[string]string{"runId": handle.RunID}, "")

	// Stream events from the run handle.
	for event := range handle.Events {
		sendEvent(event.Type, event.Data)
	}
	handle.Cancel()
}

// handleChatAbort processes a chat.abort request.
func (h *WebSocketHandler) handleChatAbort(_ context.Context, msg wsMessage, sendRes func(string, bool, any, string)) {
	var params struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil || params.SessionID == "" {
		sendRes(msg.ID, false, nil, "missing sessionId")
		return
	}

	stopped := h.api.AbortRun(params.SessionID)
	sendRes(msg.ID, true, map[string]bool{"stopped": stopped}, "")
}

// handleChatHistory processes a chat.history request.
func (h *WebSocketHandler) handleChatHistory(msg wsMessage, sendRes func(string, bool, any, string)) {
	var params struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil || params.SessionID == "" {
		sendRes(msg.ID, false, nil, "missing sessionId")
		return
	}

	messages := h.api.GetSessionMessages(params.SessionID)
	sendRes(msg.ID, true, messages, "")
}

// StreamEvent adapts the WebSocket event format to the existing copilot.StreamEvent.
func adaptStreamEvent(event webui.StreamEvent) wsMessage {
	payload, _ := json.Marshal(event.Data)
	return wsMessage{
		Type:    "event",
		Event:   event.Type,
		Payload: payload,
	}
}

// unused but shows intent:
var _ = copilot.AgentEvent{}
