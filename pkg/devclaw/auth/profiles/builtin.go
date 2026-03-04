package profiles

import (
	"fmt"
	"strings"
)

// BuiltinProviders contains metadata for built-in authentication providers.
var BuiltinProviders = map[string]*ProviderMetadata{
	// LLM Providers
	"openai": {
		Name:        "openai",
		Label:       "OpenAI",
		Description: "OpenAI API (GPT-4, GPT-3.5)",
		Modes:       []AuthMode{ModeAPIKey},
		EnvKey:      "OPENAI_API_KEY",
		Website:     "https://platform.openai.com/api-keys",
	},
	"anthropic": {
		Name:        "anthropic",
		Label:       "Anthropic",
		Description: "Anthropic API (Claude)",
		Modes:       []AuthMode{ModeAPIKey, ModeOAuth},
		EnvKey:      "ANTHROPIC_API_KEY",
		Website:     "https://console.anthropic.com/settings/keys",
		Scopes:      []string{"openid", "email", "profile"},
	},
	"gemini": {
		Name:        "gemini",
		Label:       "Google Gemini",
		Description: "Google Gemini API",
		Modes:       []AuthMode{ModeOAuth, ModeAPIKey},
		EnvKey:      "GOOGLE_API_KEY",
		Website:     "https://ai.google.dev/",
		OAuthEndpoint: OAuthEndpoint{
			AuthURL:   "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL:  "https://oauth2.googleapis.com/token",
			Scopes:    []string{"openid", "email", "profile", "https://www.googleapis.com/auth/cloud-platform"},
		},
	},
	"groq": {
		Name:        "groq",
		Label:       "Groq",
		Description: "Groq API (fast inference)",
		Modes:       []AuthMode{ModeAPIKey},
		EnvKey:      "GROQ_API_KEY",
		Website:     "https://console.groq.com/keys",
	},
	"mistral": {
		Name:        "mistral",
		Label:       "Mistral AI",
		Description: "Mistral AI API",
		Modes:       []AuthMode{ModeAPIKey},
		EnvKey:      "MISTRAL_API_KEY",
		Website:     "https://console.mistral.ai/api-keys/",
	},
	"cohere": {
		Name:        "cohere",
		Label:       "Cohere",
		Description: "Cohere API",
		Modes:       []AuthMode{ModeAPIKey},
		EnvKey:      "COHERE_API_KEY",
		Website:     "https://dashboard.cohere.com/api-keys",
	},
	"xai": {
		Name:        "xai",
		Label:       "xAI",
		Description: "xAI API (Grok)",
		Modes:       []AuthMode{ModeAPIKey},
		EnvKey:      "XAI_API_KEY",
		Website:     "https://console.x.ai/",
	},
	"deepseek": {
		Name:        "deepseek",
		Label:       "DeepSeek",
		Description: "DeepSeek API",
		Modes:       []AuthMode{ModeAPIKey},
		EnvKey:      "DEEPSEEK_API_KEY",
		Website:     "https://platform.deepseek.com/api_keys",
	},
	"qwen": {
		Name:        "qwen",
		Label:       "Qwen (Alibaba)",
		Description: "Qwen API (Alibaba Cloud)",
		Modes:       []AuthMode{ModeOAuth, ModeAPIKey},
		EnvKey:      "QWEN_API_KEY",
		Website:     "https://dashscope.aliyun.com/",
		OAuthEndpoint: OAuthEndpoint{
			Scopes: []string{"openid", "email", "profile"},
		},
	},
	"minimax": {
		Name:        "minimax",
		Label:       "MiniMax",
		Description: "MiniMax API",
		Modes:       []AuthMode{ModeOAuth, ModeAPIKey},
		EnvKey:      "MINIMAX_API_KEY",
		Website:     "https://www.minimaxi.com/",
		OAuthEndpoint: OAuthEndpoint{
			Scopes: []string{"openid", "email", "profile"},
		},
	},

}

// ProviderMetadata contains metadata about an authentication provider.
type ProviderMetadata struct {
	// Name is the provider identifier.
	Name string `json:"name"`

	// Label is a human-readable name.
	Label string `json:"label"`

	// Description explains what the provider is for.
	Description string `json:"description"`

	// Modes lists the supported authentication modes.
	Modes []AuthMode `json:"modes"`

	// EnvKey is the environment variable name for API keys.
	EnvKey string `json:"env_key,omitempty"`

	// Website is the URL for obtaining credentials.
	Website string `json:"website,omitempty"`

	// Scopes are the default OAuth scopes.
	Scopes []string `json:"scopes,omitempty"`

	// OAuthEndpoint contains OAuth endpoint configuration.
	OAuthEndpoint OAuthEndpoint `json:"oauth_endpoint,omitempty"`

	// ParentProvider indicates this is a child of another provider.
	ParentProvider string `json:"parent_provider,omitempty"`
}

// OAuthEndpoint contains OAuth endpoint URLs.
type OAuthEndpoint struct {
	AuthURL  string   `json:"auth_url,omitempty"`
	TokenURL string   `json:"token_url,omitempty"`
	Scopes   []string `json:"scopes,omitempty"`
}

// GetProvider returns metadata for a provider by name.
func GetProvider(name string) (*ProviderMetadata, bool) {
	p, ok := BuiltinProviders[strings.ToLower(name)]
	return p, ok
}

// GetProviderWithScopes returns metadata for a provider with resolved OAuth scopes.
func GetProviderWithScopes(name string) (*ProviderMetadata, bool) {
	meta, ok := BuiltinProviders[strings.ToLower(name)]
	if !ok {
		return nil, false
	}

	// Clone to avoid modifying original
	clone := *meta
	return &clone, true
}

// ListProviders returns all built-in providers.
func ListProviders() []*ProviderMetadata {
	providers := make([]*ProviderMetadata, 0, len(BuiltinProviders))
	for _, p := range BuiltinProviders {
		providers = append(providers, p)
	}
	return providers
}

// ListProvidersByMode returns providers that support a specific mode.
func ListProvidersByMode(mode AuthMode) []*ProviderMetadata {
	var result []*ProviderMetadata
	for _, p := range BuiltinProviders {
		for _, m := range p.Modes {
			if m == mode {
				result = append(result, p)
				break
			}
		}
	}
	return result
}

// IsValidProvider checks if a provider name is valid.
func IsValidProvider(name string) bool {
	_, ok := BuiltinProviders[strings.ToLower(name)]
	return ok
}

// SupportsMode checks if a provider supports a specific authentication mode.
func SupportsMode(provider string, mode AuthMode) bool {
	p, ok := GetProvider(provider)
	if !ok {
		return false
	}
	for _, m := range p.Modes {
		if m == mode {
			return true
		}
	}
	return false
}

// GetOAuthScopes returns the OAuth scopes for a provider and service.
func GetOAuthScopes(provider string, service string) ([]string, error) {
	p, ok := GetProvider(provider)
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}

	// If specific service requested, look for sub-provider
	if service != "" {
		subProvider := fmt.Sprintf("%s-%s", provider, service)
		if sub, ok := GetProvider(subProvider); ok {
			if len(sub.OAuthEndpoint.Scopes) > 0 {
				return sub.OAuthEndpoint.Scopes, nil
			}
			if len(sub.Scopes) > 0 {
				return sub.Scopes, nil
			}
		}
	}

	if len(p.OAuthEndpoint.Scopes) > 0 {
		return p.OAuthEndpoint.Scopes, nil
	}
	if len(p.Scopes) > 0 {
		return p.Scopes, nil
	}

	return nil, fmt.Errorf("no OAuth scopes defined for provider %s", provider)
}

// GetSuggestedProfileNames returns suggested profile names for a provider.
func GetSuggestedProfileNames(provider string) []string {
	_, ok := GetProvider(provider)
	if !ok {
		return []string{"default"}
	}
	return []string{"default", "work", "personal"}
}

// RegisterCustomProvider registers a custom provider at runtime.
func RegisterCustomProvider(metadata *ProviderMetadata) error {
	if metadata.Name == "" {
		return fmt.Errorf("provider name is required")
	}

	name := strings.ToLower(metadata.Name)
	if _, exists := BuiltinProviders[name]; exists {
		return fmt.Errorf("provider %s already exists", name)
	}

	BuiltinProviders[name] = metadata
	return nil
}
