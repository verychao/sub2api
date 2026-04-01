package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	platformUsageStatusOK             = "ok"
	platformUsageStatusInvalidKey     = "invalid_key"
	platformUsageStatusRequestFailed  = "request_failed"
	platformUsageStatusInvalidResp    = "invalid_response"
	platformUsageDefaultTimeout       = 8 * time.Second
	platformUsageAllowedStalenessSecs = 900
)

var invalidKeyRegexp = regexp.MustCompile(`(?i)invalid api key|incorrect api key|unauthorized|forbidden|token.*invalid|key.*invalid`)

type PlatformUsageSummary struct {
	ProviderName     string     `json:"provider_name,omitempty"`
	BaseURL          string     `json:"base_url,omitempty"`
	Remaining        *float64   `json:"remaining,omitempty"`
	Unit             string     `json:"unit,omitempty"`
	Status           string     `json:"status,omitempty"`
	IsValid          *bool      `json:"is_valid,omitempty"`
	Message          string     `json:"message,omitempty"`
	UpdatedAt        *time.Time `json:"updated_at,omitempty"`
	Source           string     `json:"source,omitempty"`
	AllowedStaleness int        `json:"allowed_staleness_seconds,omitempty"`
}

type PlatformUsageFetcher struct {
	httpUpstream HTTPUpstream
}

func NewPlatformUsageFetcher(httpUpstream HTTPUpstream) *PlatformUsageFetcher {
	return &PlatformUsageFetcher{httpUpstream: httpUpstream}
}

func (f *PlatformUsageFetcher) GetByAccount(ctx context.Context, account *Account) (*PlatformUsageSummary, error) {
	if f == nil || f.httpUpstream == nil || account == nil {
		return nil, nil
	}
	if account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey {
		return nil, nil
	}

	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	baseURL := strings.TrimSpace(account.GetBaseURL())
	if apiKey == "" || baseURL == "" {
		return nil, nil
	}

	parsedBase, err := url.Parse(baseURL)
	if err != nil {
		return &PlatformUsageSummary{
			ProviderName:     account.Name,
			BaseURL:          baseURL,
			Status:           platformUsageStatusInvalidResp,
			Message:          fmt.Sprintf("invalid base_url: %v", err),
			Source:           "sub2api_builtin",
			AllowedStaleness: platformUsageAllowedStalenessSecs,
		}, nil
	}

	usageURL := parsedBase.ResolveReference(&url.URL{Path: "/v1/usage"}).String()
	ctxWithTimeout, cancel := context.WithTimeout(ctx, platformUsageDefaultTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctxWithTimeout, http.MethodGet, usageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build usage request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := f.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return buildPlatformUsageSummary(account, baseURL, platformUsageStatusRequestFailed, nil, nil, nil, err.Error()), nil
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return buildPlatformUsageSummary(account, baseURL, platformUsageStatusInvalidResp, nil, nil, nil, fmt.Sprintf("read response failed: %v", err)), nil
	}

	var payload any
	if len(body) > 0 {
		if err := json.Unmarshal(body, &payload); err != nil {
			return buildPlatformUsageSummary(account, baseURL, platformUsageStatusInvalidResp, nil, nil, nil, fmt.Sprintf("upstream did not return valid JSON: %s", trimPlatformUsageMessage(string(body)))), nil
		}
	} else {
		payload = map[string]any{}
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || detectInvalidKeyMessage(payload) {
		isValid := false
		unit := "USD"
		return buildPlatformUsageSummary(account, baseURL, platformUsageStatusInvalidKey, nil, &unit, &isValid, extractPlatformUsageMessage(payload, fmt.Sprintf("upstream rejected API key with HTTP %d", resp.StatusCode))), nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		unit := "USD"
		return buildPlatformUsageSummary(account, baseURL, platformUsageStatusRequestFailed, nil, &unit, nil, extractPlatformUsageMessage(payload, fmt.Sprintf("upstream returned HTTP %d", resp.StatusCode))), nil
	}

	remaining, unit, err := normalizePlatformUsagePayload(payload, "USD")
	if err != nil {
		return buildPlatformUsageSummary(account, baseURL, platformUsageStatusInvalidResp, nil, nil, nil, err.Error()), nil
	}

	isValid := true
	return buildPlatformUsageSummary(account, baseURL, platformUsageStatusOK, &remaining, &unit, &isValid, ""), nil
}

func buildPlatformUsageSummary(account *Account, baseURL string, status string, remaining *float64, unit *string, isValid *bool, message string) *PlatformUsageSummary {
	now := time.Now().UTC()
	resolvedUnit := "USD"
	if unit != nil && strings.TrimSpace(*unit) != "" {
		resolvedUnit = strings.TrimSpace(*unit)
	}
	return &PlatformUsageSummary{
		ProviderName:     account.Name,
		BaseURL:          normalizePlatformUsageBaseURL(baseURL),
		Remaining:        remaining,
		Unit:             resolvedUnit,
		Status:           status,
		IsValid:          isValid,
		Message:          strings.TrimSpace(message),
		UpdatedAt:        &now,
		Source:           "sub2api_builtin",
		AllowedStaleness: platformUsageAllowedStalenessSecs,
	}
}

func normalizePlatformUsageBaseURL(baseURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimSpace(baseURL)
	}
	return parsed.Scheme + "://" + parsed.Host
}

func detectInvalidKeyMessage(payload any) bool {
	for _, candidate := range collectPlatformUsageMessages(payload) {
		if invalidKeyRegexp.MatchString(candidate) {
			return true
		}
	}
	return false
}

func extractPlatformUsageMessage(payload any, fallback string) string {
	for _, candidate := range collectPlatformUsageMessages(payload) {
		if candidate != "" {
			return candidate
		}
	}
	return fallback
}

func collectPlatformUsageMessages(payload any) []string {
	var out []string
	appendString := func(v any) {
		if s, ok := v.(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
	}

	appendString(payload)
	obj, ok := payload.(map[string]any)
	if !ok {
		return out
	}
	appendString(obj["message"])
	appendString(obj["msg"])
	appendString(obj["detail"])
	appendString(obj["error"])
	if nested, ok := obj["error"].(map[string]any); ok {
		appendString(nested["message"])
		appendString(nested["type"])
		appendString(nested["code"])
	}
	return out
}

func normalizePlatformUsagePayload(payload any, defaultUnit string) (float64, string, error) {
	obj, ok := payload.(map[string]any)
	if !ok {
		return 0, "", fmt.Errorf("upstream payload is not a JSON object")
	}

	directPaths := [][]string{{"remaining"}, {"balance"}, {"available"}, {"available_balance"}, {"remaining_balance"}, {"remainingBalance"}, {"credit_balance"}, {"quota"}, {"credits"}, {"data", "remaining"}, {"data", "balance"}, {"data", "available"}, {"data", "available_balance"}, {"data", "remaining_balance"}, {"data", "credit_balance"}, {"data", "quota"}, {"data", "credits"}, {"credit_grants", "total_available"}}
	for _, path := range directPaths {
		if value, ok := lookupPlatformUsageNumber(obj, path); ok {
			return roundPlatformCurrency(value), pickPlatformUsageUnit(obj, defaultUnit), nil
		}
	}

	hardLimit, hardOK := firstPlatformUsageNumber(obj, [][]string{{"hard_limit_usd"}, {"data", "hard_limit_usd"}, {"monthlyLimit"}, {"monthly_limit"}, {"credit_grants", "total_granted"}})
	totalUsage, usageOK := firstPlatformUsageNumber(obj, [][]string{{"total_usage"}, {"data", "total_usage"}, {"used"}, {"data", "used"}, {"credit_grants", "total_used"}})
	if hardOK && usageOK {
		remaining := hardLimit - totalUsage
		if remaining < 0 {
			remaining = 0
		}
		return roundPlatformCurrency(remaining), pickPlatformUsageUnit(obj, defaultUnit), nil
	}

	return 0, "", fmt.Errorf("unable to map upstream payload to a remaining balance field")
}

func firstPlatformUsageNumber(payload map[string]any, paths [][]string) (float64, bool) {
	for _, path := range paths {
		if v, ok := lookupPlatformUsageNumber(payload, path); ok {
			return v, true
		}
	}
	return 0, false
}

func lookupPlatformUsageNumber(payload any, path []string) (float64, bool) {
	current := payload
	for _, key := range path {
		obj, ok := current.(map[string]any)
		if !ok {
			return 0, false
		}
		current, ok = obj[key]
		if !ok {
			return 0, false
		}
	}
	return coercePlatformUsageNumber(current)
}

func coercePlatformUsageNumber(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		parsed, err := v.Float64()
		return parsed, err == nil
	case string:
		parsed, err := json.Number(strings.TrimSpace(v)).Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func pickPlatformUsageUnit(payload map[string]any, defaultUnit string) string {
	paths := [][]string{{"unit"}, {"currency"}, {"data", "unit"}, {"data", "currency"}}
	for _, path := range paths {
		current := any(payload)
		for _, key := range path {
			obj, ok := current.(map[string]any)
			if !ok {
				current = nil
				break
			}
			current = obj[key]
		}
		if s, ok := current.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return defaultUnit
}

func roundPlatformCurrency(value float64) float64 {
	return float64(int(value*100+0.5)) / 100
}

func trimPlatformUsageMessage(value string) string {
	sanitized := strings.Join(strings.Fields(value), " ")
	if len(sanitized) <= 140 {
		return sanitized
	}
	return sanitized[:140] + "..."
}
