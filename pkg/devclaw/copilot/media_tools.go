// Package copilot – media_tools.go registers tools for image understanding
// (describe_image), audio transcription (transcribe_audio), and the unified
// send_media tool for sending images, audio, video, and documents.
package copilot

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jholhewres/devclaw/pkg/devclaw/channels"
	"github.com/jholhewres/devclaw/pkg/devclaw/media"
)

// RegisterMediaTools registers describe_image and transcribe_audio tools
// when the LLM client and config support them.
// If VisionProviders or TranscriptionProviders are configured in MediaConfig,
// a MediaRegistry is used for priority-based fallback across multiple providers.
func RegisterMediaTools(executor *ToolExecutor, llmClient *LLMClient, cfg *Config, logger *slog.Logger) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	mediaCfg := cfg.Media.Effective()

	// Build registry if multi-provider configs are present.
	var registry *MediaRegistry
	if len(mediaCfg.VisionProviders) > 0 || len(mediaCfg.TranscriptionProviders) > 0 {
		registry = NewMediaRegistry(
			mediaCfg.VisionProviders,
			mediaCfg.TranscriptionProviders,
			mediaCfg.ConcurrencyLimit,
			logger,
		)
		SetGlobalMediaRegistry(registry)
	}

	if mediaCfg.VisionEnabled && llmClient != nil {
		registerDescribeImageTool(executor, llmClient, mediaCfg, registry, logger)
	}

	if mediaCfg.TranscriptionEnabled && llmClient != nil {
		registerTranscribeAudioTool(executor, llmClient, mediaCfg, registry, logger)
	}
}

func registerDescribeImageTool(executor *ToolExecutor, llm *LLMClient, media MediaConfig, registry *MediaRegistry, logger *slog.Logger) {
	executor.Register(
		MakeToolDefinition("describe_image", "Analyze an image from base64 data. Use real base64 from screenshot captures, not placeholders.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"image_base64": map[string]any{
					"type":        "string",
					"description": "The actual base64-encoded image data string. Must be real base64 data from a screenshot or image capture, NOT a placeholder or description.",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "Optional question or instruction about the image (e.g. 'What is in this image?', 'Extract the text')",
				},
				"detail": map[string]any{
					"type":        "string",
					"description": "Vision detail level: 'auto', 'low', or 'high'. Default: auto",
					"enum":        []string{"auto", "low", "high"},
				},
				"mime_type": map[string]any{
					"type":        "string",
					"description": "MIME type of the image (e.g. image/jpeg, image/png). Default: image/jpeg",
				},
			},
			"required": []string{"image_base64"},
		}),
		func(ctx context.Context, args map[string]any) (any, error) {
			imageBase64, _ := args["image_base64"].(string)
			if imageBase64 == "" {
				return nil, fmt.Errorf("image_base64 is required")
			}

			// Decode to verify and check size.
			decoded, err := base64.StdEncoding.DecodeString(imageBase64)
			if err != nil {
				return nil, fmt.Errorf("invalid base64 image: %w", err)
			}

			if int64(len(decoded)) > media.MaxImageSize {
				return nil, fmt.Errorf("image too large: %d bytes (max %d)", len(decoded), media.MaxImageSize)
			}

			prompt, _ := args["prompt"].(string)
			if prompt == "" {
				prompt = "Describe this image in detail. Include any text visible in the image."
			}

			detail, _ := args["detail"].(string)
			if detail == "" {
				detail = media.VisionDetail
			}
			if detail == "" {
				detail = "auto"
			}

			mimeType, _ := args["mime_type"].(string)
			if mimeType == "" {
				mimeType = "image/jpeg"
			}

			logger.Debug("describing image",
				"size_bytes", len(decoded),
				"prompt", truncate(prompt, 50),
			)

			// Use multi-provider registry if available, otherwise single provider.
			if registry != nil && registry.HasVisionProviders() {
				desc, err := registry.DescribeImageWithFallback(ctx, "", imageBase64, mimeType, prompt, detail)
				if err != nil {
					logger.Error("vision registry failed", "error", err)
					return nil, fmt.Errorf("vision API: %w", err)
				}
				return desc, nil
			}

			desc, err := llm.CompleteWithVision(ctx, "", imageBase64, mimeType, prompt, detail)
			if err != nil {
				logger.Error("vision API failed", "error", err)
				return nil, fmt.Errorf("vision API: %w", err)
			}

			return desc, nil
		},
	)
	logger.Debug("registered describe_image tool")
}

func registerTranscribeAudioTool(executor *ToolExecutor, llm *LLMClient, media MediaConfig, registry *MediaRegistry, logger *slog.Logger) {
	executor.RegisterHidden(
		MakeToolDefinition("transcribe_audio", "Transcribe audio/voice to text from base64-encoded audio data.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"audio_base64": map[string]any{
					"type":        "string",
					"description": "Base64-encoded audio data",
				},
				"filename": map[string]any{
					"type":        "string",
					"description": "Filename hint for format (e.g. audio.ogg, voice.mp3, recording.webm). Helps the API detect format.",
				},
			},
			"required": []string{"audio_base64"},
		}),
		func(ctx context.Context, args map[string]any) (any, error) {
			audioBase64, _ := args["audio_base64"].(string)
			if audioBase64 == "" {
				return nil, fmt.Errorf("audio_base64 is required")
			}

			decoded, err := base64.StdEncoding.DecodeString(audioBase64)
			if err != nil {
				return nil, fmt.Errorf("invalid base64 audio: %w", err)
			}

			if int64(len(decoded)) > media.MaxAudioSize {
				return nil, fmt.Errorf("audio too large: %d bytes (max %d)", len(decoded), media.MaxAudioSize)
			}

			filename, _ := args["filename"].(string)
			if filename == "" {
				filename = "audio.webm"
			}

			logger.Debug("transcribing audio",
				"size_bytes", len(decoded),
				"filename", filename,
			)

			// Use multi-provider registry if available, otherwise single provider.
			if registry != nil && registry.HasTranscriptionProviders() {
				transcript, err := registry.TranscribeAudioWithFallback(ctx, decoded, filename, media.TranscriptionModel, media)
				if err != nil {
					logger.Error("transcription registry failed", "error", err)
					return nil, fmt.Errorf("transcription: %w", err)
				}
				return transcript, nil
			}

			transcript, err := llm.TranscribeAudio(ctx, decoded, filename, media.TranscriptionModel, media)
			if err != nil {
				logger.Error("transcription failed", "error", err)
				return nil, fmt.Errorf("transcription: %w", err)
			}

			return transcript, nil
		},
	)
	logger.Debug("registered transcribe_audio tool")
}

// RegisterNativeMediaTools registers the unified send_media tool
// for the LLM to send media (images, audio, documents, video) to users
// through the channel manager. mediaSvc may be nil — in that case only
// file_path is supported (no media_id or url); this ensures the
// send_media tool is always available when channels are connected.
func RegisterNativeMediaTools(executor *ToolExecutor, mediaSvc *media.MediaService, channelMgr *channels.Manager, logger *slog.Logger) {
	if channelMgr == nil {
		return
	}

	registerSendMediaTool(executor, mediaSvc, channelMgr, logger)
}

// validMediaTypes maps user-facing type strings to channels.MessageType.
var validMediaTypes = map[string]channels.MessageType{
	"image":    channels.MessageImage,
	"audio":    channels.MessageAudio,
	"video":    channels.MessageVideo,
	"document": channels.MessageDocument,
}

func registerSendMediaTool(executor *ToolExecutor, mediaSvc *media.MediaService, channelMgr *channels.Manager, logger *slog.Logger) {
	// Build tool description and properties based on available backends.
	desc := "Send media (image, audio, video, or document) to the current user via file path. Channel and recipient are auto-detected from the conversation context. The media type is auto-detected from the file content when not specified."
	props := map[string]any{
		"type": map[string]any{
			"type":        "string",
			"description": "Media type: image, audio, video, or document. If omitted, auto-detected from the file MIME type.",
			"enum":        []string{"image", "audio", "video", "document"},
		},
		"file_path": map[string]any{
			"type":        "string",
			"description": "Local file path to the media on the server",
		},
		"caption": map[string]any{
			"type":        "string",
			"description": "Optional caption text for the media",
		},
		"channel": map[string]any{
			"type":        "string",
			"description": "Target channel (e.g., whatsapp, telegram). Auto-detected from context if omitted.",
		},
		"to": map[string]any{
			"type":        "string",
			"description": "Recipient phone number or chat ID. Auto-detected from context if omitted.",
		},
	}

	// Expose media_id and url only when the full media service is available.
	if mediaSvc != nil {
		desc = "Send media (image, audio, video, or document) to the current user via media_id, file path, or URL. Channel and recipient are auto-detected from the conversation context. The media type is auto-detected from the file content when not specified."
		props["media_id"] = map[string]any{
			"type":        "string",
			"description": "ID of previously uploaded media (from /api/media upload)",
		}
		props["url"] = map[string]any{
			"type":        "string",
			"description": "URL to download the media from",
		}
	}

	executor.Register(
		MakeToolDefinition("send_media", desc, map[string]any{
			"type":       "object",
			"properties": props,
		}),
		func(ctx context.Context, args map[string]any) (any, error) {
			// Resolve media source.
			var data []byte
			var mimeType, filename string
			var err error

			if mediaSvc != nil {
				data, mimeType, filename, err = mediaSvc.ResolveMediaSource(ctx, args)
			} else {
				data, mimeType, filename, err = resolveFilePathOnly(args)
			}
			if err != nil {
				return nil, fmt.Errorf("resolving media source: %w", err)
			}

			// Determine media type: explicit > auto-detect from MIME
			var msgType channels.MessageType
			if typeStr, _ := args["type"].(string); typeStr != "" {
				mt, ok := validMediaTypes[typeStr]
				if !ok {
					return nil, fmt.Errorf("invalid media type %q: must be image, audio, video, or document", typeStr)
				}
				msgType = mt
			} else {
				// Auto-detect from MIME type
				msgType = channels.MessageType(media.CategorizeType(mimeType))
			}

			// Get parameters
			caption, _ := args["caption"].(string)
			channelName, _ := args["channel"].(string)
			to, _ := args["to"].(string)

			// Auto-resolve channel and recipient from delivery context.
			if channelName == "" || to == "" {
				dt := DeliveryTargetFromContext(ctx)
				if channelName == "" {
					channelName = dt.Channel
				}
				if to == "" {
					to = dt.ChatID
				}
			}

			if channelName == "" {
				return nil, fmt.Errorf("could not determine target channel — provide 'channel' parameter or ensure delivery context is set")
			}
			if to == "" {
				return nil, fmt.Errorf("could not determine recipient — provide 'to' parameter or ensure delivery context is set")
			}

			// Web UI path: save to media store and emit SSE event.
			if channelName == "webui" && mediaSvc != nil {
				stored, uploadErr := mediaSvc.Upload(ctx, media.UploadRequest{
					Data:      data,
					Filename:  filename,
					MimeType:  mimeType,
					Channel:   "webui",
					SessionID: to,
					Temporary: true,
				})
				if uploadErr != nil {
					return nil, fmt.Errorf("storing media for web UI: %w", uploadErr)
				}

				if emitter := MediaEmitterFromContext(ctx); emitter != nil {
					emitter(MediaEvent{
						ID:       stored.ID,
						URL:      mediaSvc.URL(stored.ID),
						Type:     string(msgType),
						MimeType: mimeType,
						Filename: filename,
						Size:     stored.Size,
						Caption:  caption,
					})
				} else {
					logger.Warn("media stored but no emitter available for web UI delivery",
						"media_id", stored.ID,
						"session", to,
					)
				}

				logger.Info("media emitted to web UI",
					"media_id", stored.ID,
					"type", msgType,
					"filename", filename,
					"size", stored.Size,
				)

				return map[string]any{
					"status":   "sent",
					"type":     string(msgType),
					"filename": filename,
					"size":     stored.Size,
					"channel":  channelName,
					"media_id": stored.ID,
				}, nil
			}

			// Build MediaMessage
			msg := &channels.MediaMessage{
				Type:     msgType,
				Data:     data,
				MimeType: mimeType,
				Filename: filename,
				Caption:  caption,
			}

			// Send via channel manager
			if err := channelMgr.SendMedia(ctx, channelName, to, msg); err != nil {
				logger.Error("failed to send media",
					"channel", channelName,
					"to", to,
					"type", msgType,
					"error", err,
				)
				return nil, fmt.Errorf("sending media via %s: %w", channelName, err)
			}

			logger.Info("media sent",
				"channel", channelName,
				"to", to,
				"type", msgType,
				"filename", filename,
				"size", len(data),
			)

			return map[string]any{
				"status":   "sent",
				"type":     string(msgType),
				"filename": filename,
				"size":     len(data),
				"channel":  channelName,
			}, nil
		},
	)
	logger.Debug("registered send_media tool")
}

// resolveFilePathOnly reads a file from disk and detects its MIME type.
// Used as a lightweight fallback when the full MediaService is not available.
func resolveFilePathOnly(args map[string]any) ([]byte, string, string, error) {
	filePath, _ := args["file_path"].(string)
	if filePath == "" {
		return nil, "", "", fmt.Errorf("file_path is required (media_id and url require native media service to be enabled)")
	}

	clean := filepath.Clean(filePath)
	if strings.Contains(clean, "..") {
		return nil, "", "", fmt.Errorf("path traversal not allowed")
	}

	data, err := os.ReadFile(clean)
	if err != nil {
		return nil, "", "", fmt.Errorf("reading file: %w", err)
	}

	filename := filepath.Base(clean)
	mimeType := detectMimeFromDataAndName(data, filename)
	return data, mimeType, filename, nil
}

// detectMimeFromDataAndName uses http.DetectContentType with extension-based
// fallback for types that content sniffing misidentifies (e.g. PDF, OGG).
func detectMimeFromDataAndName(data []byte, filename string) string {
	detected := http.DetectContentType(data)

	// http.DetectContentType returns "application/octet-stream" for many
	// binary formats. Use extension heuristics as a fallback.
	if detected == "application/octet-stream" {
		ext := strings.ToLower(filepath.Ext(filename))
		switch ext {
		case ".pdf":
			return "application/pdf"
		case ".doc":
			return "application/msword"
		case ".docx":
			return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		case ".xls":
			return "application/vnd.ms-excel"
		case ".xlsx":
			return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		case ".mp3":
			return "audio/mpeg"
		case ".m4a":
			return "audio/mp4"
		case ".ogg":
			return "audio/ogg"
		case ".wav":
			return "audio/wav"
		}
	}

	return detected
}
