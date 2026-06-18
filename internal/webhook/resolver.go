package webhook

import (
	"strconv"
	"strings"
	"time"

	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
)

func ResolveConfig(cfg *config.AppConfig, watcher *database.Watcher) ResolvedConfig {
	resolved := ResolvedConfig{
		Enabled:               watcher != nil && watcher.WebhookEnabled,
		URL:                   strings.TrimSpace(cfg.WebhookDefaultURL),
		SigningSecret:         strings.TrimSpace(cfg.WebhookDefaultSigningSecret),
		SecretSource:          "global_default",
		Timeout:               time.Duration(max(cfg.WebhookTimeoutSec, 1)) * time.Second,
		RetrySchedule:         parseRetrySchedule(cfg.WebhookRetryScheduleSec),
		AutoPauseEnabled:      cfg.WebhookAutoPauseEnabled,
		AutoPauseAfter:        max(cfg.WebhookAutoPauseAfter, 1),
		EventRetentionDays:    max(cfg.WebhookEventRetentionDays, 1),
		DeliveryRetentionDays: max(cfg.WebhookDeliveryRetentionDays, 1),
	}
	if watcher == nil {
		return resolved
	}
	if url := strings.TrimSpace(watcher.WebhookURL); url != "" {
		resolved.URL = url
	}
	if secret := strings.TrimSpace(watcher.WebhookSigningSecret); secret != "" {
		resolved.SigningSecret = secret
		resolved.SecretSource = "watcher_override"
	}
	if watcher.WebhookAutoPauseEnabledOverride != nil {
		resolved.AutoPauseEnabled = *watcher.WebhookAutoPauseEnabledOverride
	}
	if watcher.WebhookAutoPauseAfterFailures != nil && *watcher.WebhookAutoPauseAfterFailures > 0 {
		resolved.AutoPauseAfter = *watcher.WebhookAutoPauseAfterFailures
	}
	return resolved
}

func parseRetrySchedule(raw string) []time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []time.Duration{0, 10 * time.Second, time.Minute, 5 * time.Minute}
	}
	parts := strings.Split(raw, ",")
	out := make([]time.Duration, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 0 {
			continue
		}
		out = append(out, time.Duration(n)*time.Second)
	}
	if len(out) == 0 {
		return []time.Duration{0, 10 * time.Second, time.Minute, 5 * time.Minute}
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
