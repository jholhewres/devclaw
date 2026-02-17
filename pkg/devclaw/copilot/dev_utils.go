// Package copilot – dev_utils.go implements developer utility tools:
// JSON formatting, JWT decoding, regex testing, base64 encode/decode,
// hashing, UUID generation, URL parsing, timestamp conversion, etc.
package copilot

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// RegisterDevUtilTools registers developer utility tools.
func RegisterDevUtilTools(executor *ToolExecutor) {
	// json_format
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "json_format",
			Description: "Format, validate, or minify JSON. Auto-detects if input is valid JSON.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"input":  map[string]any{"type": "string", "description": "JSON string to format"},
					"minify": map[string]any{"type": "boolean", "description": "Minify instead of pretty-print"},
				},
				"required": []string{"input"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		input, _ := args["input"].(string)
		minify, _ := args["minify"].(bool)

		var parsed any
		if err := json.Unmarshal([]byte(input), &parsed); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}

		var out []byte
		if minify {
			out, _ = json.Marshal(parsed)
		} else {
			out, _ = json.MarshalIndent(parsed, "", "  ")
		}
		return string(out), nil
	})

	// jwt_decode
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "jwt_decode",
			Description: "Decode a JWT token without verification. Shows header, payload, and expiration info.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"token": map[string]any{"type": "string", "description": "JWT token to decode"},
				},
				"required": []string{"token"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		token, _ := args["token"].(string)
		parts := strings.Split(token, ".")
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid JWT: expected 3 parts, got %d", len(parts))
		}

		header, err := decodeJWTSegment(parts[0])
		if err != nil {
			return nil, fmt.Errorf("decoding header: %w", err)
		}

		payload, err := decodeJWTSegment(parts[1])
		if err != nil {
			return nil, fmt.Errorf("decoding payload: %w", err)
		}

		// Check expiration
		var expInfo string
		if payloadMap, ok := payload.(map[string]any); ok {
			if exp, ok := payloadMap["exp"].(float64); ok {
				expTime := time.Unix(int64(exp), 0)
				if time.Now().After(expTime) {
					expInfo = fmt.Sprintf("EXPIRED at %s (%s ago)", expTime.Format(time.RFC3339), time.Since(expTime).Truncate(time.Second))
				} else {
					expInfo = fmt.Sprintf("Valid until %s (in %s)", expTime.Format(time.RFC3339), time.Until(expTime).Truncate(time.Second))
				}
			}
		}

		result := map[string]any{
			"header":  header,
			"payload": payload,
		}
		if expInfo != "" {
			result["expiration"] = expInfo
		}

		data, _ := json.MarshalIndent(result, "", "  ")
		return string(data), nil
	})

	// regex_test
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "regex_test",
			Description: "Test a regular expression against input text. Returns matches and capture groups.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{"type": "string", "description": "Regular expression pattern"},
					"input":   map[string]any{"type": "string", "description": "Text to match against"},
					"global":  map[string]any{"type": "boolean", "description": "Find all matches (default: first match only)"},
				},
				"required": []string{"pattern", "input"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		pattern, _ := args["pattern"].(string)
		input, _ := args["input"].(string)
		global, _ := args["global"].(bool)

		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex: %w", err)
		}

		result := map[string]any{
			"pattern": pattern,
			"valid":   true,
		}

		if global {
			matches := re.FindAllStringSubmatch(input, -1)
			result["match_count"] = len(matches)
			result["matches"] = matches
		} else {
			match := re.FindStringSubmatch(input)
			result["matched"] = match != nil
			result["match"] = match
			result["groups"] = re.SubexpNames()
		}

		data, _ := json.MarshalIndent(result, "", "  ")
		return string(data), nil
	})

	// base64_encode
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "base64_encode",
			Description: "Encode text to Base64.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"input": map[string]any{"type": "string", "description": "Text to encode"},
				},
				"required": []string{"input"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		input, _ := args["input"].(string)
		return base64.StdEncoding.EncodeToString([]byte(input)), nil
	})

	// base64_decode
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "base64_decode",
			Description: "Decode Base64 to text.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"input": map[string]any{"type": "string", "description": "Base64 string to decode"},
				},
				"required": []string{"input"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		input, _ := args["input"].(string)
		decoded, err := base64.StdEncoding.DecodeString(input)
		if err != nil {
			// Try URL-safe variant
			decoded, err = base64.URLEncoding.DecodeString(input)
			if err != nil {
				return nil, fmt.Errorf("invalid base64: %w", err)
			}
		}
		return string(decoded), nil
	})

	// hash
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "hash",
			Description: "Generate hash of input text: MD5, SHA-1, or SHA-256.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"input":     map[string]any{"type": "string", "description": "Text to hash"},
					"algorithm": map[string]any{"type": "string", "enum": []string{"md5", "sha1", "sha256"}, "description": "Hash algorithm (default: sha256)"},
				},
				"required": []string{"input"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		input, _ := args["input"].(string)
		algo, _ := args["algorithm"].(string)
		if algo == "" {
			algo = "sha256"
		}

		switch algo {
		case "md5":
			return fmt.Sprintf("%x", md5.Sum([]byte(input))), nil
		case "sha1":
			return fmt.Sprintf("%x", sha1.Sum([]byte(input))), nil
		case "sha256":
			return fmt.Sprintf("%x", sha256.Sum256([]byte(input))), nil
		default:
			return nil, fmt.Errorf("unsupported algorithm: %s", algo)
		}
	})

	// uuid_generate
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "uuid_generate",
			Description: "Generate one or more UUIDs (v4).",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"count": map[string]any{"type": "integer", "description": "Number of UUIDs to generate (default: 1, max: 20)"},
				},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		count := 1
		if v, ok := args["count"].(float64); ok {
			count = int(v)
			if count > 20 {
				count = 20
			}
		}

		uuids := make([]string, count)
		for i := range uuids {
			uuids[i] = uuid.New().String()
		}

		if count == 1 {
			return uuids[0], nil
		}
		return strings.Join(uuids, "\n"), nil
	})

	// url_parse
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "url_parse",
			Description: "Parse a URL into its components: scheme, host, path, query parameters, fragment.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{"type": "string", "description": "URL to parse"},
				},
				"required": []string{"url"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		rawURL, _ := args["url"].(string)
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return nil, fmt.Errorf("invalid URL: %w", err)
		}

		result := map[string]any{
			"scheme":   parsed.Scheme,
			"host":     parsed.Host,
			"hostname": parsed.Hostname(),
			"port":     parsed.Port(),
			"path":     parsed.Path,
			"fragment": parsed.Fragment,
		}

		if parsed.User != nil {
			result["user"] = parsed.User.Username()
		}

		params := map[string]any{}
		for key, values := range parsed.Query() {
			if len(values) == 1 {
				params[key] = values[0]
			} else {
				params[key] = values
			}
		}
		if len(params) > 0 {
			result["query_params"] = params
		}

		data, _ := json.MarshalIndent(result, "", "  ")
		return string(data), nil
	})

	// timestamp_convert
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "timestamp_convert",
			Description: "Convert between Unix timestamps and human-readable dates. If no input given, returns current time.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"unix": map[string]any{"type": "number", "description": "Unix timestamp (seconds) to convert to date"},
					"date": map[string]any{"type": "string", "description": "Date string (RFC3339) to convert to Unix timestamp"},
				},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		now := time.Now()

		if unix, ok := args["unix"].(float64); ok {
			t := time.Unix(int64(unix), 0)
			result := map[string]any{
				"unix":     int64(unix),
				"utc":      t.UTC().Format(time.RFC3339),
				"local":    t.Local().Format(time.RFC3339),
				"relative": time.Since(t).Truncate(time.Second).String() + " ago",
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			return string(data), nil
		}

		if date, ok := args["date"].(string); ok && date != "" {
			t, err := time.Parse(time.RFC3339, date)
			if err != nil {
				t, err = time.Parse("2006-01-02", date)
				if err != nil {
					t, err = time.Parse("2006-01-02 15:04:05", date)
					if err != nil {
						return nil, fmt.Errorf("unsupported date format (use RFC3339, YYYY-MM-DD, or YYYY-MM-DD HH:MM:SS)")
					}
				}
			}
			result := map[string]any{
				"unix":  t.Unix(),
				"utc":   t.UTC().Format(time.RFC3339),
				"local": t.Local().Format(time.RFC3339),
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			return string(data), nil
		}

		// No input: return current time
		result := map[string]any{
			"unix":  now.Unix(),
			"utc":   now.UTC().Format(time.RFC3339),
			"local": now.Local().Format(time.RFC3339),
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return string(data), nil
	})
}

// decodeJWTSegment decodes a single JWT segment (base64url -> JSON).
func decodeJWTSegment(seg string) (any, error) {
	// Pad base64url
	switch len(seg) % 4 {
	case 2:
		seg += "=="
	case 3:
		seg += "="
	}

	decoded, err := base64.URLEncoding.DecodeString(seg)
	if err != nil {
		return nil, err
	}

	var result any
	if err := json.Unmarshal(decoded, &result); err != nil {
		return string(decoded), nil
	}
	return result, nil
}
