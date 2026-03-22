package webui

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jholhewres/devclaw/pkg/devclaw/channels"
)

// handleAPIChannelInstances handles /api/channels/instances/{type}[/{id}[/action]].
// Routes:
//
//	GET    /api/channels/instances/{type}              → list instances
//	POST   /api/channels/instances/{type}              → create instance
//	DELETE /api/channels/instances/{type}/{id}          → delete instance
//	GET    /api/channels/instances/{type}/{id}/status   → instance status
//	GET    /api/channels/instances/{type}/{id}/qr       → QR stream (SSE)
//	POST   /api/channels/instances/{type}/{id}/qr       → request QR
//	POST   /api/channels/instances/{type}/{id}/disconnect → disconnect
func (s *Server) handleAPIChannelInstances(w http.ResponseWriter, r *http.Request) {
	adapter, ok := s.api.(*AssistantAdapter)
	if !ok {
		http.Error(w, "adapter not available", http.StatusInternalServerError)
		return
	}

	// Parse path: /api/channels/instances/{type}[/{id}[/{action}]]
	path := strings.TrimPrefix(r.URL.Path, "/api/channels/instances/")
	parts := strings.SplitN(path, "/", 3)

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "channel type required", http.StatusBadRequest)
		return
	}

	channelType := parts[0]

	// P2: Validate channel type against whitelist.
	if !channels.ValidChannelTypes[channelType] {
		http.Error(w, "unknown channel type", http.StatusBadRequest)
		return
	}

	// GET/POST /api/channels/instances/{type} — list or create
	if len(parts) == 1 || (len(parts) == 2 && parts[1] == "") {
		switch r.Method {
		case http.MethodGet:
			s.handleListInstances(w, channelType, adapter)
		case http.MethodPost:
			s.handleCreateInstance(w, r, channelType, adapter)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	instanceID := parts[1]

	// P0: Validate instance ID to prevent path traversal and injection.
	if err := channels.ValidateInstanceID(instanceID); err != nil {
		http.Error(w, "invalid instance ID", http.StatusBadRequest)
		return
	}

	// DELETE /api/channels/instances/{type}/{id}
	if len(parts) == 2 || (len(parts) == 3 && parts[2] == "") {
		if r.Method == http.MethodDelete {
			s.handleDeleteInstance(w, channelType, instanceID, adapter)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// /api/channels/instances/{type}/{id}/{action}
	if len(parts) == 3 {
		action := parts[2]
		switch action {
		case "status":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			s.handleInstanceStatus(w, channelType, instanceID, adapter)
		case "qr":
			s.handleInstanceQR(w, r, channelType, instanceID, adapter)
		case "disconnect":
			s.handleInstanceDisconnect(w, r, channelType, instanceID, adapter)
		default:
			http.Error(w, "unknown action", http.StatusNotFound)
		}
		return
	}

	http.Error(w, "not found", http.StatusNotFound)
}

func (s *Server) handleListInstances(w http.ResponseWriter, channelType string, adapter *AssistantAdapter) {
	if adapter.ListChannelInstancesFn == nil {
		writeJSON(w, http.StatusOK, []ChannelInstanceInfo{})
		return
	}
	instances := adapter.ListChannelInstancesFn(channelType)
	if instances == nil {
		instances = []ChannelInstanceInfo{}
	}
	writeJSON(w, http.StatusOK, instances)
}

func (s *Server) handleCreateInstance(w http.ResponseWriter, r *http.Request, channelType string, adapter *AssistantAdapter) {
	if adapter.CreateChannelInstanceFn == nil {
		http.Error(w, "instance creation not supported", http.StatusNotImplemented)
		return
	}

	var body struct {
		InstanceID string         `json:"instance_id"`
		Config     map[string]any `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.InstanceID == "" {
		http.Error(w, "instance_id required", http.StatusBadRequest)
		return
	}

	// P0: Validate instance ID in request body.
	if err := channels.ValidateInstanceID(body.InstanceID); err != nil {
		http.Error(w, "invalid instance_id: must be 1-64 alphanumeric, underscore, or hyphen characters", http.StatusBadRequest)
		return
	}

	if err := adapter.CreateChannelInstanceFn(channelType, body.InstanceID, body.Config); err != nil {
		// P0: Don't expose internal error details to the client.
		slog.Error("failed to create channel instance",
			"channel_type", channelType,
			"instance_id", body.InstanceID,
			"error", err,
		)
		http.Error(w, "failed to create instance", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "created", "instance_id": body.InstanceID})
}

func (s *Server) handleDeleteInstance(w http.ResponseWriter, channelType, instanceID string, adapter *AssistantAdapter) {
	if adapter.DeleteChannelInstanceFn == nil {
		http.Error(w, "instance deletion not supported", http.StatusNotImplemented)
		return
	}

	if err := adapter.DeleteChannelInstanceFn(channelType, instanceID); err != nil {
		slog.Error("failed to delete channel instance",
			"channel_type", channelType,
			"instance_id", instanceID,
			"error", err,
		)
		http.Error(w, "failed to delete instance", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleInstanceStatus(w http.ResponseWriter, channelType, instanceID string, adapter *AssistantAdapter) {
	// WhatsApp has a rich status endpoint.
	if channelType == "whatsapp" {
		if adapter.GetWhatsAppStatusByInstanceFn == nil {
			http.Error(w, "not available", http.StatusNotImplemented)
			return
		}
		status := adapter.GetWhatsAppStatusByInstanceFn(instanceID)
		writeJSON(w, http.StatusOK, status)
		return
	}

	// Generic fallback: derive status from the channel instance list.
	if adapter.ListChannelInstancesFn != nil {
		for _, inst := range adapter.ListChannelInstancesFn(channelType) {
			if inst.InstanceID == instanceID || (instanceID == "" && inst.InstanceID == "") {
				state := "disconnected"
				if inst.Connected {
					state = "connected"
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"connected": inst.Connected,
					"state":     state,
					"needs_qr":  inst.NeedsQR,
				})
				return
			}
		}
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}

	http.Error(w, "status not supported for this channel type", http.StatusNotImplemented)
}

func (s *Server) handleInstanceQR(w http.ResponseWriter, r *http.Request, channelType, instanceID string, adapter *AssistantAdapter) {
	if channelType != "whatsapp" {
		http.Error(w, "QR not supported for this channel type", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// SSE stream
		if adapter.SubscribeWhatsAppQRByInstanceFn == nil {
			http.Error(w, "not available", http.StatusNotImplemented)
			return
		}
		ch, unsub := adapter.SubscribeWhatsAppQRByInstanceFn(instanceID)
		defer unsub()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		// P1: Respect client disconnect to prevent goroutine leaks.
		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-ch:
				if !ok {
					return
				}
				writeSSE(w, flusher, evt.Type, evt)
			}
		}

	case http.MethodPost:
		// Request new QR
		if adapter.RequestWhatsAppQRByInstanceFn == nil {
			http.Error(w, "not available", http.StatusNotImplemented)
			return
		}
		if err := adapter.RequestWhatsAppQRByInstanceFn(instanceID); err != nil {
			slog.Error("failed to request QR for instance",
				"instance_id", instanceID,
				"error", err,
			)
			http.Error(w, "failed to request QR code", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "requested"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleInstanceDisconnect(w http.ResponseWriter, r *http.Request, channelType, instanceID string, adapter *AssistantAdapter) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	switch channelType {
	case "whatsapp":
		if adapter.DisconnectWhatsAppByInstanceFn == nil {
			http.Error(w, "not available", http.StatusNotImplemented)
			return
		}
		if err := adapter.DisconnectWhatsAppByInstanceFn(instanceID); err != nil {
			slog.Error("failed to disconnect whatsapp instance",
				"instance_id", instanceID,
				"error", err,
			)
			http.Error(w, "failed to disconnect instance", http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, "disconnect not supported for this channel type", http.StatusNotImplemented)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}
