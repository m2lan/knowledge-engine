package embedder

import (
	"fmt"
	"net/http"
	"time"
)

// EmbedderConfig holds configuration for creating an embedder.
type EmbedderConfig struct {
	Provider   string
	Model      string
	APIKey     string
	BaseURL    string
	Dimensions int
	HTTPClient *http.Client
}

// Factory creates an Embedder from a config.
type Factory func(cfg EmbedderConfig) (Embedder, error)

var registry = map[string]Factory{}

// Register adds an embedding provider factory under the given name.
func Register(provider string, f Factory) {
	registry[provider] = f
}

// Providers returns the list of registered provider names.
func Providers() []string {
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	return names
}

// New creates an Embedder using the registered factory for cfg.Provider.
func New(cfg EmbedderConfig) (Embedder, error) {
	f, ok := registry[cfg.Provider]
	if !ok {
		return nil, fmt.Errorf("unknown embedding provider %q, registered: %v", cfg.Provider, Providers())
	}
	return f(cfg)
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

func init() {
	Register("openai", func(cfg EmbedderConfig) (Embedder, error) {
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("openai embedding provider requires api_key")
		}
		return NewOpenAIEmbedder(cfg.APIKey, cfg.BaseURL, cfg.Model, cfg.Dimensions, cfg.HTTPClient), nil
	})

	Register("gemini", func(cfg EmbedderConfig) (Embedder, error) {
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("gemini embedding provider requires api_key")
		}
		return NewGeminiEmbedder(cfg.APIKey, cfg.Model, cfg.Dimensions, cfg.HTTPClient), nil
	})

	Register("ollama", func(cfg EmbedderConfig) (Embedder, error) {
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:11434/v1"
		}
		model := cfg.Model
		if model == "" {
			model = "nomic-embed-text"
		}
		dims := cfg.Dimensions
		if dims == 0 {
			dims = 768
		}
		client := cfg.HTTPClient
		if client == nil {
			client = defaultHTTPClient()
		}
		return &OpenAIEmbedder{
			apiKey:     cfg.APIKey,
			baseURL:    baseURL,
			model:      model,
			dimensions: dims,
			client:     client,
		}, nil
	})
}
