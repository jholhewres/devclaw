// Package whatsapp – events.go processes incoming WhatsApp events from
// whatsmeow and converts them into unified DevClaw IncomingMessage types.
package whatsapp

import (
	"fmt"
	"strings"

	"github.com/jholhewres/devclaw/pkg/devclaw/channels"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
)

// handleEvent is the main whatsmeow event dispatcher.
func (w *WhatsApp) handleEvent(rawEvt interface{}) {
	switch evt := rawEvt.(type) {
	case *events.Message:
		w.handleMessageEvt(evt)

	case *events.Receipt:
		w.handleReceipt(evt)

	case *events.Connected:
		w.connected.Store(true)
		w.errorCount.Store(0)
		w.logger.Info("whatsapp: connected")

	case *events.Disconnected:
		w.connected.Store(false)
		w.logger.Warn("whatsapp: disconnected")

	case *events.StreamReplaced:
		w.connected.Store(false)
		w.logger.Warn("whatsapp: stream replaced (another device connected)")

	case *events.LoggedOut:
		w.connected.Store(false)
		w.logger.Error("whatsapp: logged out, session invalidated",
			"reason", evt.Reason)

	case *events.TemporaryBan:
		w.logger.Error("whatsapp: temporary ban",
			"code", evt.Code,
			"expire", evt.Expire)

	case *events.HistorySync:
		w.logger.Debug("whatsapp: history sync received")

	case *events.PushName:
		w.logger.Debug("whatsapp: push name update",
			"jid", evt.JID, "name", evt.NewPushName)
	}
}

// handleMessageEvt processes an incoming WhatsApp message event.
func (w *WhatsApp) handleMessageEvt(evt *events.Message) {
	// Skip messages from self.
	if evt.Info.IsFromMe {
		return
	}

	// Skip status broadcasts.
	if evt.Info.Chat.Server == "broadcast" {
		return
	}

	// Check group/DM filtering.
	isGroup := evt.Info.IsGroup
	if isGroup && !w.cfg.RespondToGroups {
		return
	}
	if !isGroup && !w.cfg.RespondToDMs {
		return
	}

	// Resolve sender JID — WhatsApp may use LID (Linked Identity) format
	// instead of phone numbers. We resolve to phone JID for access control.
	senderJID := evt.Info.Sender
	resolvedSender := senderJID.String()
	if senderJID.Server == "lid" && w.client != nil && w.client.Store != nil {
		if altJID, err := w.client.Store.GetAltJID(w.ctx, senderJID); err == nil && !altJID.IsEmpty() {
			resolvedSender = altJID.String()
			w.logger.Debug("whatsapp: resolved LID to phone",
				"lid", senderJID.String(), "phone", resolvedSender)
		}
	}

	// Resolve chat JID (for DMs, chat may also be in LID format).
	chatJID := evt.Info.Chat
	resolvedChat := chatJID.String()
	if chatJID.Server == "lid" && w.client != nil && w.client.Store != nil {
		if altJID, err := w.client.Store.GetAltJID(w.ctx, chatJID); err == nil && !altJID.IsEmpty() {
			resolvedChat = altJID.String()
		}
	}

	// Build the incoming message.
	msg := &channels.IncomingMessage{
		ID:        string(evt.Info.ID),
		Channel:   "whatsapp",
		From:      resolvedSender,
		FromName:  evt.Info.PushName,
		ChatID:    resolvedChat,
		IsGroup:   isGroup,
		Timestamp: evt.Info.Timestamp,
		Metadata: map[string]any{
			"sender_jid":  senderJID.String(),
			"sender_lid":  senderJID.String(),
			"sender_phone": resolvedSender,
			"chat_jid":    chatJID.String(),
			"push_name":   evt.Info.PushName,
		},
	}

	// Extract message content by type.
	w.extractMessageContent(evt.Message, msg)

	// Handle quoted/reply messages.
	w.extractQuotedMessage(evt.Message, msg)

	// Auto-read if configured.
	if w.cfg.AutoRead {
		go func() {
			_ = w.MarkRead(w.ctx, msg.ChatID, []string{msg.ID})
		}()
	}

	// Emit the message.
	w.emitMessage(msg)
}

// extractMessageContent extracts the text/media content from a WhatsApp message.
func (w *WhatsApp) extractMessageContent(waMsg *waE2E.Message, msg *channels.IncomingMessage) {
	if waMsg == nil {
		return
	}

	// Text message (simple conversation).
	if waMsg.Conversation != nil {
		msg.Type = channels.MessageText
		msg.Content = waMsg.GetConversation()
		return
	}

	// Extended text message (with preview, formatting, etc.).
	if ext := waMsg.ExtendedTextMessage; ext != nil {
		msg.Type = channels.MessageText
		msg.Content = ext.GetText()
		return
	}

	// Image message.
	if img := waMsg.ImageMessage; img != nil {
		msg.Type = channels.MessageImage
		msg.Content = img.GetCaption()
		msg.Media = &channels.MediaInfo{
			Type:          channels.MessageImage,
			MimeType:      img.GetMimetype(),
			FileSize:      img.GetFileLength(),
			Caption:       img.GetCaption(),
			Width:         img.GetWidth(),
			Height:        img.GetHeight(),
			URL:           img.GetURL(),
			DirectPath:    img.GetDirectPath(),
			MediaKey:      img.GetMediaKey(),
			FileSHA256:    img.GetFileSHA256(),
			FileEncSHA256: img.GetFileEncSHA256(),
		}
		return
	}

	// Audio message (voice note or audio file).
	if audio := waMsg.AudioMessage; audio != nil {
		msg.Type = channels.MessageAudio
		msg.Content = "[audio]"
		if audio.GetPTT() {
			msg.Content = "[voice note]"
		}
		msg.Media = &channels.MediaInfo{
			Type:          channels.MessageAudio,
			MimeType:      audio.GetMimetype(),
			FileSize:      audio.GetFileLength(),
			Duration:      audio.GetSeconds(),
			URL:           audio.GetURL(),
			DirectPath:    audio.GetDirectPath(),
			MediaKey:      audio.GetMediaKey(),
			FileSHA256:    audio.GetFileSHA256(),
			FileEncSHA256: audio.GetFileEncSHA256(),
		}
		return
	}

	// Video message.
	if video := waMsg.VideoMessage; video != nil {
		msg.Type = channels.MessageVideo
		msg.Content = video.GetCaption()
		msg.Media = &channels.MediaInfo{
			Type:          channels.MessageVideo,
			MimeType:      video.GetMimetype(),
			FileSize:      video.GetFileLength(),
			Caption:       video.GetCaption(),
			Duration:      video.GetSeconds(),
			Width:         video.GetWidth(),
			Height:        video.GetHeight(),
			URL:           video.GetURL(),
			DirectPath:    video.GetDirectPath(),
			MediaKey:      video.GetMediaKey(),
			FileSHA256:    video.GetFileSHA256(),
			FileEncSHA256: video.GetFileEncSHA256(),
		}
		return
	}

	// Document message.
	if doc := waMsg.DocumentMessage; doc != nil {
		msg.Type = channels.MessageDocument
		msg.Content = doc.GetCaption()
		if msg.Content == "" {
			msg.Content = fmt.Sprintf("[document: %s]", doc.GetFileName())
		}
		msg.Media = &channels.MediaInfo{
			Type:          channels.MessageDocument,
			MimeType:      doc.GetMimetype(),
			Filename:      doc.GetFileName(),
			FileSize:      doc.GetFileLength(),
			Caption:       doc.GetCaption(),
			URL:           doc.GetURL(),
			DirectPath:    doc.GetDirectPath(),
			MediaKey:      doc.GetMediaKey(),
			FileSHA256:    doc.GetFileSHA256(),
			FileEncSHA256: doc.GetFileEncSHA256(),
		}
		return
	}

	// Sticker message.
	if sticker := waMsg.StickerMessage; sticker != nil {
		msg.Type = channels.MessageSticker
		msg.Content = "[sticker]"
		msg.Media = &channels.MediaInfo{
			Type:          channels.MessageSticker,
			MimeType:      sticker.GetMimetype(),
			FileSize:      sticker.GetFileLength(),
			Width:         sticker.GetWidth(),
			Height:        sticker.GetHeight(),
			URL:           sticker.GetURL(),
			DirectPath:    sticker.GetDirectPath(),
			MediaKey:      sticker.GetMediaKey(),
			FileSHA256:    sticker.GetFileSHA256(),
			FileEncSHA256: sticker.GetFileEncSHA256(),
		}
		return
	}

	// Location message.
	if loc := waMsg.LocationMessage; loc != nil {
		msg.Type = channels.MessageLocation
		msg.Content = fmt.Sprintf("[location: %.6f, %.6f]",
			loc.GetDegreesLatitude(), loc.GetDegreesLongitude())
		msg.Location = &channels.LocationInfo{
			Latitude:  loc.GetDegreesLatitude(),
			Longitude: loc.GetDegreesLongitude(),
			Name:      loc.GetName(),
			Address:   loc.GetAddress(),
			URL:       loc.GetURL(),
			AccuracyM: loc.GetAccuracyInMeters(),
		}
		return
	}

	// Live location.
	if loc := waMsg.LiveLocationMessage; loc != nil {
		msg.Type = channels.MessageLocation
		msg.Content = fmt.Sprintf("[live location: %.6f, %.6f]",
			loc.GetDegreesLatitude(), loc.GetDegreesLongitude())
		msg.Location = &channels.LocationInfo{
			Latitude:  loc.GetDegreesLatitude(),
			Longitude: loc.GetDegreesLongitude(),
			AccuracyM: loc.GetAccuracyInMeters(),
		}
		return
	}

	// Contact message.
	if contact := waMsg.ContactMessage; contact != nil {
		msg.Type = channels.MessageContact
		msg.Content = fmt.Sprintf("[contact: %s]", contact.GetDisplayName())
		msg.Contact = &channels.ContactInfo{
			DisplayName: contact.GetDisplayName(),
			VCard:       contact.GetVcard(),
		}
		return
	}

	// Reaction message.
	if reaction := waMsg.ReactionMessage; reaction != nil {
		msg.Type = channels.MessageReaction
		msg.Content = reaction.GetText()
		msg.Reaction = &channels.ReactionInfo{
			Emoji:     reaction.GetText(),
			MessageID: reaction.GetKey().GetID(),
			Remove:    reaction.GetText() == "",
		}
		return
	}

	// Fallback: unknown message type.
	msg.Type = channels.MessageText
	msg.Content = "[unsupported message type]"
}

// extractQuotedMessage extracts reply/quoted context from a message.
func (w *WhatsApp) extractQuotedMessage(waMsg *waE2E.Message, msg *channels.IncomingMessage) {
	if waMsg == nil {
		return
	}

	// Collect context info from any message type that supports quoting.
	var ctxInfo *waE2E.ContextInfo

	switch {
	case waMsg.ExtendedTextMessage != nil:
		ctxInfo = waMsg.ExtendedTextMessage.GetContextInfo()
	case waMsg.ImageMessage != nil:
		ctxInfo = waMsg.ImageMessage.GetContextInfo()
	case waMsg.AudioMessage != nil:
		ctxInfo = waMsg.AudioMessage.GetContextInfo()
	case waMsg.VideoMessage != nil:
		ctxInfo = waMsg.VideoMessage.GetContextInfo()
	case waMsg.DocumentMessage != nil:
		ctxInfo = waMsg.DocumentMessage.GetContextInfo()
	case waMsg.StickerMessage != nil:
		ctxInfo = waMsg.StickerMessage.GetContextInfo()
	}

	if ctxInfo == nil {
		return
	}

	if ctxInfo.StanzaID != nil {
		msg.ReplyTo = ctxInfo.GetStanzaID()
	}
	if quoted := ctxInfo.QuotedMessage; quoted != nil {
		msg.QuotedContent = extractQuotedText(quoted)
	}
}

// extractQuotedText gets the text from a quoted message.
func extractQuotedText(quoted *waE2E.Message) string {
	if quoted.Conversation != nil {
		return quoted.GetConversation()
	}
	if ext := quoted.ExtendedTextMessage; ext != nil {
		return ext.GetText()
	}
	if img := quoted.ImageMessage; img != nil {
		return "[image] " + img.GetCaption()
	}
	if vid := quoted.VideoMessage; vid != nil {
		return "[video] " + vid.GetCaption()
	}
	if doc := quoted.DocumentMessage; doc != nil {
		return "[document: " + doc.GetFileName() + "]"
	}
	if audio := quoted.AudioMessage; audio != nil {
		if audio.GetPTT() {
			return "[voice note]"
		}
		return "[audio]"
	}
	return "[message]"
}

// handleReceipt processes read/delivery receipts.
func (w *WhatsApp) handleReceipt(evt *events.Receipt) {
	switch evt.Type {
	case types.ReceiptTypeRead:
		w.logger.Debug("whatsapp: message read",
			"from", evt.Chat, "ids", evt.MessageIDs)
	case types.ReceiptTypeDelivered:
		w.logger.Debug("whatsapp: message delivered",
			"from", evt.Chat, "ids", evt.MessageIDs)
	}
}

// ---------- Helpers ----------

// parseJID converts a string JID to types.JID.
// Accepts formats: "5511999999999" or "5511999999999@s.whatsapp.net"
// or group IDs like "123456789-1234@g.us".
func parseJID(s string) (types.JID, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return types.JID{}, fmt.Errorf("empty JID")
	}

	// Already a full JID with server.
	if strings.Contains(s, "@") {
		return types.ParseJID(s)
	}

	// Bare phone number — add @s.whatsapp.net.
	// Remove any non-digit characters.
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)

	if len(digits) < 10 {
		return types.JID{}, fmt.Errorf("phone number too short: %s", s)
	}

	return types.NewJID(digits, types.DefaultUserServer), nil
}
