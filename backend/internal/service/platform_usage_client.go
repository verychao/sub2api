package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	httppool "github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

type PlatformUsageSummary struct {
	ProviderName     string     `json:"provider_name,omitempty"`
	BaseURL          string     `json:"base_url,omitempty"`
	Remaining        *float64   `json:"remaining,omitempty"`
	Unit             string     `json:"unit,omitempty"`
	Status           string     `json:"status,omitempty"`
	IsValid          *bool      `json:"is_valid,omitempty"`
	Message          string     `json:"message,omitempty"`
	GeneratedAt      *time.Time `json:"generated_at,omitempty"`
	UpdatedAt        *time.Time `json:"updated_at,omitempty"`
	Stale            bool       `json:"stale,omitempty"`
	Source           string     `json:"source,omitempty"`
	AllowedStaleness int        `json:"allowed_staleness_seconds,omitempty"`
}

type platformUsagePayload struct {
	Success     bool                    `json:"success"`
	GeneratedAt string                  `json:"generatedAt"`
	Platforms   []platformUsageProvider `json:"platforms"`
}

type platformUsageProvider struct {
	Name      string   `json:"name"`
	BaseURL   string   `json:"baseUrl"`
	Remaining *float64 `json:"remaining"`
	Unit      string   `json:"unit"`
	IsValid   *bool    `json:"isValid"`
	Status    string   `json:"status"`
	UpdatedAt string   `json:"updatedAt"`
	Message   string   `json:"message"`
}

type cachedPlatformUsage struct {
	generatedAt time.Time
	providers   map[string]platformUsageProvider
}

type PlatformUsageClient struct {
	enabled          bool
	url              string
	timeout          time.Duration
	allowedStaleness time.Duration
	httpClient       *http.Client

	mu    sync.RWMutex
	cache *cachedPlatformUsage
}

func NewPlatformUsageClient(cfg *config.Config) *PlatformUsageClient {
	if cfg == nil || !cfg.PlatformUsage.Enabled || cfg.PlatformUsage.URL == "" {
		return &PlatformUsageClient{enabled: false}
	}

	return &PlatformUsageClient{
		enabled:          true,
		url:              cfg.PlatformUsage.URL,
		timeout:          time.Duration(cfg.PlatformUsage.TimeoutSeconds) * time.Second,
		allowedStaleness: time.Duration(cfg.PlatformUsage.AllowedStaleness) * time.Second,
		httpClient:       httppool.GetSharedClient(),
	}
}

func (c *PlatformUsageClient) Enabled() bool {
	return c != nil && c.enabled
}

func (c *PlatformUsageClient) GetByAccount(ctx context.Context, account *Account) (*PlatformUsageSummary, error) {
	if !c.Enabled() || account == nil {
		return nil, nil
	}

	provider, generatedAt, err := c.lookup(ctx, account.Name)
	if err != nil {
		return nil, err
	}
	if provider == nil {
		return nil, nil
	}

	summary := &PlatformUsageSummary{
		ProviderName:     provider.Name,
		BaseURL:          provider.BaseURL,
		Remaining:        provider.Remaining,
		Unit:             strings.TrimSpace(provider.Unit),
		Status:           strings.TrimSpace(provider.Status),
		IsValid:          provider.IsValid,
		Message:          strings.TrimSpace(provider.Message),
		Source:           "platform_usage_service",
		AllowedStaleness: int(c.allowedStaleness.Seconds()),
	}
	if summary.Unit == "" {
		summary.Unit = "USD"
	}
	if !generatedAt.IsZero() {
		t := generatedAt
		summary.GeneratedAt = &t
		if c.allowedStaleness > 0 && time.Since(generatedAt) > c.allowedStaleness {
			summary.Stale = true
		}
	}
	if ts, err := parseOptionalTimestamp(provider.UpdatedAt); err == nil {
		summary.UpdatedAt = ts
	}

	return summary, nil
}

func (c *PlatformUsageClient) lookup(ctx context.Context, accountName string) (*platformUsageProvider, time.Time, error) {
	cache, err := c.fetch(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	if cache == nil {
		return nil, time.Time{}, nil
	}

	key := strings.ToLower(strings.TrimSpace(accountName))
	provider, ok := cache.providers[key]
	if !ok {
		return nil, cache.generatedAt, nil
	}
	copyProvider := provider
	return &copyProvider, cache.generatedAt, nil
}

func (c *PlatformUsageClient) fetch(ctx context.Context) (*cachedPlatformUsage, error) {
	c.mu.RLock()
	if c.cache != nil && (c.allowedStaleness <= 0 || time.Since(c.cache.generatedAt) <= c.allowedStaleness) {
		cached := c.cache
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()

	ctxWithTimeout, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctxWithTimeout, http.MethodGet, c.url, nil)
	if err != nil {
		return nil, fmt.Errorf("build platform usage request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request platform usage service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("platform usage service returned status %d", resp.StatusCode)
	}

	var payload platformUsagePayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode platform usage response: %w", err)
	}

	generatedAt, _ := time.Parse(time.RFC3339, strings.TrimSpace(payload.GeneratedAt))
	providers := make(map[string]platformUsageProvider, len(payload.Platforms))
	for _, item := range payload.Platforms {
		name := strings.ToLower(strings.TrimSpace(item.Name))
		if name == "" {
			continue
		}
		providers[name] = item
	}

	cache := &cachedPlatformUsage{generatedAt: generatedAt, providers: providers}
	c.mu.Lock()
	c.cache = cache
	c.mu.Unlock()
	return cache, nil
}

func parseOptionalTimestamp(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
