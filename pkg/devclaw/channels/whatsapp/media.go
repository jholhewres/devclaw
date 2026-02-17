// Package whatsapp – media.go handles media upload, download, and
// message building for images, audio, video, documents, and stickers.
package whatsapp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/channels"

	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	waCommon "go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// buildTextMessage creates a WhatsApp text message proto.
func buildTextMessage(text, replyTo string) *waE2E.Message {
	// Simple conversation for non-reply.
	if replyTo == "" {
		return &waE2E.Message{
			Conversation: proto.String(text),
		}
	}

	// Extended text with context info for reply.
	return &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String(text),
			ContextInfo: &waE2E.ContextInfo{
				StanzaID: proto.String(replyTo),
			},
		},
	}
}

// buildReactionMessage creates a reaction message proto.
func buildReactionMessage(messageID string, chatJID types.JID, emoji string) *waE2E.Message {
	return &waE2E.Message{
		ReactionMessage: &waE2E.ReactionMessage{
			Key: &waCommon.MessageKey{
				RemoteJID: proto.String(chatJID.String()),
				ID:        proto.String(messageID),
				FromMe:    proto.Bool(false),
			},
			Text:              proto.String(emoji),
			SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
		},
	}
}

// buildMediaMessage creates a WhatsApp media message proto.
// Handles upload to WhatsApp servers with encryption.
func (w *WhatsApp) buildMediaMessage(ctx context.Context, media *channels.MediaMessage) (*waE2E.Message, error) {
	// Get media data.
	data, err := w.resolveMediaData(ctx, media)
	if err != nil {
		return nil, fmt.Errorf("resolving media data: %w", err)
	}

	// Detect MIME type if not set.
	mimeType := media.MimeType
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}

	switch media.Type {
	case channels.MessageImage:
		return w.uploadImage(ctx, data, mimeType, media)
	case channels.MessageAudio:
		return w.uploadAudio(ctx, data, mimeType, media)
	case channels.MessageVideo:
		return w.uploadVideo(ctx, data, mimeType, media)
	case channels.MessageDocument:
		return w.uploadDocument(ctx, data, mimeType, media)
	case channels.MessageSticker:
		return w.uploadSticker(ctx, data, mimeType, media)
	default:
		return nil, fmt.Errorf("unsupported media type: %s", media.Type)
	}
}

// uploadImage uploads and builds an image message.
func (w *WhatsApp) uploadImage(ctx context.Context, data []byte, mimeType string, media *channels.MediaMessage) (*waE2E.Message, error) {
	resp, err := w.client.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return nil, fmt.Errorf("uploading image: %w", err)
	}

	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			URL:           &resp.URL,
			Mimetype:      proto.String(mimeType),
			Caption:       proto.String(media.Caption),
			FileSHA256:    resp.FileSHA256,
			FileEncSHA256: resp.FileEncSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			MediaKey:      resp.MediaKey,
			DirectPath:    &resp.DirectPath,
			Width:         proto.Uint32(media.Width),
			Height:        proto.Uint32(media.Height),
		},
	}
	return msg, nil
}

// uploadAudio uploads and builds an audio message.
func (w *WhatsApp) uploadAudio(ctx context.Context, data []byte, mimeType string, media *channels.MediaMessage) (*waE2E.Message, error) {
	resp, err := w.client.Upload(ctx, data, whatsmeow.MediaAudio)
	if err != nil {
		return nil, fmt.Errorf("uploading audio: %w", err)
	}

	// Voice messages use audio/ogg; codecs=opus.
	ptt := strings.Contains(mimeType, "ogg") || strings.Contains(mimeType, "opus")

	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:           &resp.URL,
			Mimetype:      proto.String(mimeType),
			FileSHA256:    resp.FileSHA256,
			FileEncSHA256: resp.FileEncSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			MediaKey:      resp.MediaKey,
			DirectPath:    &resp.DirectPath,
			Seconds:       proto.Uint32(media.Duration),
			PTT:           proto.Bool(ptt),
		},
	}
	return msg, nil
}

// uploadVideo uploads and builds a video message.
func (w *WhatsApp) uploadVideo(ctx context.Context, data []byte, mimeType string, media *channels.MediaMessage) (*waE2E.Message, error) {
	resp, err := w.client.Upload(ctx, data, whatsmeow.MediaVideo)
	if err != nil {
		return nil, fmt.Errorf("uploading video: %w", err)
	}

	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			URL:           &resp.URL,
			Mimetype:      proto.String(mimeType),
			Caption:       proto.String(media.Caption),
			FileSHA256:    resp.FileSHA256,
			FileEncSHA256: resp.FileEncSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			MediaKey:      resp.MediaKey,
			DirectPath:    &resp.DirectPath,
			Seconds:       proto.Uint32(media.Duration),
			Width:         proto.Uint32(media.Width),
			Height:        proto.Uint32(media.Height),
		},
	}
	return msg, nil
}

// uploadDocument uploads and builds a document message.
func (w *WhatsApp) uploadDocument(ctx context.Context, data []byte, mimeType string, media *channels.MediaMessage) (*waE2E.Message, error) {
	resp, err := w.client.Upload(ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		return nil, fmt.Errorf("uploading document: %w", err)
	}

	filename := media.Filename
	if filename == "" {
		filename = "document"
	}

	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           &resp.URL,
			Mimetype:      proto.String(mimeType),
			Title:         proto.String(filename),
			FileName:      proto.String(filename),
			Caption:       proto.String(media.Caption),
			FileSHA256:    resp.FileSHA256,
			FileEncSHA256: resp.FileEncSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			MediaKey:      resp.MediaKey,
			DirectPath:    &resp.DirectPath,
		},
	}
	return msg, nil
}

// uploadSticker uploads and builds a sticker message.
func (w *WhatsApp) uploadSticker(ctx context.Context, data []byte, mimeType string, media *channels.MediaMessage) (*waE2E.Message, error) {
	resp, err := w.client.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return nil, fmt.Errorf("uploading sticker: %w", err)
	}

	msg := &waE2E.Message{
		StickerMessage: &waE2E.StickerMessage{
			URL:           &resp.URL,
			Mimetype:      proto.String(mimeType),
			FileSHA256:    resp.FileSHA256,
			FileEncSHA256: resp.FileEncSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			MediaKey:      resp.MediaKey,
			DirectPath:    &resp.DirectPath,
			Width:         proto.Uint32(media.Width),
			Height:        proto.Uint32(media.Height),
			IsAnimated:    proto.Bool(strings.Contains(mimeType, "webp")),
		},
	}
	return msg, nil
}

// downloadMediaFromInfo downloads media using MediaInfo from an incoming message.
func (w *WhatsApp) downloadMediaFromInfo(ctx context.Context, info *channels.MediaInfo) ([]byte, string, error) {
	if w.client == nil {
		return nil, "", fmt.Errorf("client not connected")
	}

	// Build a downloadable message based on media type.
	var downloadable whatsmeow.DownloadableMessage

	switch info.Type {
	case channels.MessageImage:
		downloadable = &waE2E.ImageMessage{
			URL:           proto.String(info.URL),
			DirectPath:    proto.String(info.DirectPath),
			MediaKey:      info.MediaKey,
			FileSHA256:    info.FileSHA256,
			FileEncSHA256: info.FileEncSHA256,
			FileLength:    proto.Uint64(info.FileSize),
			Mimetype:      proto.String(info.MimeType),
		}
	case channels.MessageAudio:
		downloadable = &waE2E.AudioMessage{
			URL:           proto.String(info.URL),
			DirectPath:    proto.String(info.DirectPath),
			MediaKey:      info.MediaKey,
			FileSHA256:    info.FileSHA256,
			FileEncSHA256: info.FileEncSHA256,
			FileLength:    proto.Uint64(info.FileSize),
			Mimetype:      proto.String(info.MimeType),
		}
	case channels.MessageVideo:
		downloadable = &waE2E.VideoMessage{
			URL:           proto.String(info.URL),
			DirectPath:    proto.String(info.DirectPath),
			MediaKey:      info.MediaKey,
			FileSHA256:    info.FileSHA256,
			FileEncSHA256: info.FileEncSHA256,
			FileLength:    proto.Uint64(info.FileSize),
			Mimetype:      proto.String(info.MimeType),
		}
	case channels.MessageDocument:
		downloadable = &waE2E.DocumentMessage{
			URL:           proto.String(info.URL),
			DirectPath:    proto.String(info.DirectPath),
			MediaKey:      info.MediaKey,
			FileSHA256:    info.FileSHA256,
			FileEncSHA256: info.FileEncSHA256,
			FileLength:    proto.Uint64(info.FileSize),
			Mimetype:      proto.String(info.MimeType),
		}
	case channels.MessageSticker:
		downloadable = &waE2E.StickerMessage{
			URL:           proto.String(info.URL),
			DirectPath:    proto.String(info.DirectPath),
			MediaKey:      info.MediaKey,
			FileSHA256:    info.FileSHA256,
			FileEncSHA256: info.FileEncSHA256,
			FileLength:    proto.Uint64(info.FileSize),
			Mimetype:      proto.String(info.MimeType),
		}
	default:
		return nil, "", fmt.Errorf("unsupported media type for download: %s", info.Type)
	}

	data, err := w.client.Download(ctx, downloadable)
	if err != nil {
		return nil, "", fmt.Errorf("downloading media: %w", err)
	}

	return data, info.MimeType, nil
}

// SaveMediaToFile downloads and saves media to disk.
func (w *WhatsApp) SaveMediaToFile(ctx context.Context, msg *channels.IncomingMessage) (string, error) {
	if msg.Media == nil {
		return "", fmt.Errorf("message has no media")
	}

	data, mimeType, err := w.downloadMediaFromInfo(ctx, msg.Media)
	if err != nil {
		return "", err
	}

	// Build filename.
	ext := extensionFromMime(mimeType)
	filename := msg.Media.Filename
	if filename == "" {
		filename = fmt.Sprintf("%s_%s%s", msg.Channel, msg.ID, ext)
	}

	path := filepath.Join(w.cfg.MediaDir, filename)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}

	return path, nil
}

// resolveMediaData gets the raw bytes from a MediaMessage.
func (w *WhatsApp) resolveMediaData(_ context.Context, media *channels.MediaMessage) ([]byte, error) {
	// If raw data is provided, use it directly.
	if len(media.Data) > 0 {
		return media.Data, nil
	}

	// If URL is provided, download it.
	if media.URL != "" {
		return downloadURL(media.URL)
	}

	// If filename looks like a local path, read it.
	if media.Filename != "" {
		return os.ReadFile(media.Filename)
	}

	return nil, fmt.Errorf("no media data, URL, or file path provided")
}

// downloadURL fetches data from a URL.
func downloadURL(url string) ([]byte, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// extensionFromMime maps MIME types to file extensions.
func extensionFromMime(mimeType string) string {
	switch {
	case strings.Contains(mimeType, "jpeg"), strings.Contains(mimeType, "jpg"):
		return ".jpg"
	case strings.Contains(mimeType, "png"):
		return ".png"
	case strings.Contains(mimeType, "webp"):
		return ".webp"
	case strings.Contains(mimeType, "gif"):
		return ".gif"
	case strings.Contains(mimeType, "ogg"):
		return ".ogg"
	case strings.Contains(mimeType, "opus"):
		return ".opus"
	case strings.Contains(mimeType, "mp3"), strings.Contains(mimeType, "mpeg"):
		return ".mp3"
	case strings.Contains(mimeType, "mp4"):
		return ".mp4"
	case strings.Contains(mimeType, "webm"):
		return ".webm"
	case strings.Contains(mimeType, "pdf"):
		return ".pdf"
	case strings.Contains(mimeType, "zip"):
		return ".zip"
	default:
		return ".bin"
	}
}
