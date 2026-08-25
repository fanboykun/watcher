package api

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/fanboykun/watcher/internal/webhook"
)

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

func validatePort(port string) error {
	if port == "" {
		return fmt.Errorf("api_port cannot be empty")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("api_port must be a valid number between 1 and 65535")
	}
	return nil
}

func validateOptionalWebhookSigningSecret(secret string) error {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil
	}
	if _, err := webhook.NewStandardWebhook(secret); err != nil {
		return fmt.Errorf("webhook signing secret must be valid base64 secret material; optional whsec_ prefix is allowed")
	}
	return nil
}

func (h *Handler) validateWebhookDefaultsDependency(next *config.AppConfig) error {
	if h == nil || h.db == nil || next == nil {
		return nil
	}

	if strings.TrimSpace(next.WebhookDefaultURL) == "" {
		var count int64
		if err := h.db.Model(&database.Watcher{}).
			Where("webhook_enabled = ? AND (webhook_url IS NULL OR TRIM(webhook_url) = '')", true).
			Count(&count).Error; err != nil {
			return fmt.Errorf("count watchers using default webhook url: %w", err)
		}
		if count > 0 {
			return fmt.Errorf("cannot clear webhook_default_url while %d enabled watcher(s) still inherit it", count)
		}
	}

	if strings.TrimSpace(next.WebhookDefaultSigningSecret) == "" {
		var count int64
		if err := h.db.Model(&database.Watcher{}).
			Where("webhook_enabled = ? AND (webhook_signing_secret IS NULL OR TRIM(webhook_signing_secret) = '')", true).
			Count(&count).Error; err != nil {
			return fmt.Errorf("count watchers using default webhook signing secret: %w", err)
		}
		if count > 0 {
			return fmt.Errorf("cannot clear webhook_default_signing_secret while %d enabled watcher(s) still inherit it", count)
		}
	}

	return nil
}

func maskToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if len(token) <= 8 {
		return "********"
	}
	return token[:4] + strings.Repeat("*", len(token)-8) + token[len(token)-4:]
}

func (h *Handler) selfServiceName() string {
	if h.appCfg != nil && strings.TrimSpace(h.appCfg.WatcherServiceName) != "" {
		return strings.TrimSpace(h.appCfg.WatcherServiceName)
	}
	return "app-watcher"
}
