package webui

import (
	"fmt"
	"net/http"
	"strings"
)

// handleAPIWhatsAppQR routes WhatsApp QR-related requests.
//   GET  /api/channels/whatsapp/status → connection status
//   GET  /api/channels/whatsapp/qr     → SSE stream of QR events
//   POST /api/channels/whatsapp/qr     → request a new QR code
func (s *Server) handleAPIWhatsAppQR(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/channels/whatsapp")
	adapter, ok := s.api.(*AssistantAdapter)
	if !ok || adapter == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "assistant not available",
		})
		return
	}

	switch {
	case path == "/status" && r.Method == http.MethodGet:
		s.handleWhatsAppStatus(w, r, adapter)
	case path == "/qr" && r.Method == http.MethodGet:
		s.handleWhatsAppQRStream(w, r, adapter)
	case path == "/qr" && r.Method == http.MethodPost:
		s.handleWhatsAppQRRequest(w, r, adapter)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

// handleWhatsAppStatus returns current WhatsApp connection status.
func (s *Server) handleWhatsAppStatus(w http.ResponseWriter, _ *http.Request, adapter *AssistantAdapter) {
	if adapter.GetWhatsAppStatusFn == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "WhatsApp status not available",
		})
		return
	}
	writeJSON(w, http.StatusOK, adapter.GetWhatsAppStatusFn())
}

// handleWhatsAppQRStream opens an SSE connection that streams QR code events.
// Events: "qr" (with code string), "success", "timeout", "error".
func (s *Server) handleWhatsAppQRStream(w http.ResponseWriter, r *http.Request, adapter *AssistantAdapter) {
	if adapter.SubscribeWhatsAppQRFn == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "WhatsApp QR streaming not available",
		})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "streaming not supported",
		})
		return
	}

	// Subscribe to QR events.
	events, unsubscribe := adapter.SubscribeWhatsAppQRFn()
	defer unsubscribe()

	// SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()

	// Send current status immediately as first event.
	if adapter.GetWhatsAppStatusFn != nil {
		status := adapter.GetWhatsAppStatusFn()
		writeSSE(w, flusher, "status", status)
	}

	// Stream events until client disconnects or channel closes.
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-events:
			if !ok {
				// Channel closed — send final event and exit.
				fmt.Fprintf(w, "event: close\ndata: {}\n\n")
				flusher.Flush()
				return
			}
			writeSSE(w, flusher, evt.Type, evt)
		}
	}
}

// handleWhatsAppQRRequest triggers a new QR code generation.
func (s *Server) handleWhatsAppQRRequest(w http.ResponseWriter, _ *http.Request, adapter *AssistantAdapter) {
	if adapter.RequestWhatsAppQRFn == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "WhatsApp QR request not available",
		})
		return
	}

	if err := adapter.RequestWhatsAppQRFn(); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"message": "QR code generation started",
	})
}
