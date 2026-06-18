package webhook

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"gorm.io/gorm"
)

type Logger interface {
	Warn(string, ...any)
}

type Dispatcher struct {
	db      *gorm.DB
	appCfg  *config.AppConfig
	log     Logger
	service *Service
	trigger chan struct{}
}

func NewDispatcher(db *gorm.DB, appCfg *config.AppConfig, log Logger, service *Service, trigger chan struct{}) *Dispatcher {
	return &Dispatcher{db: db, appCfg: appCfg, log: log, service: service, trigger: trigger}
}

func (d *Dispatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-d.trigger:
		}
		d.cleanupExpired()
		d.dispatchOnce(ctx)
	}
}

func (d *Dispatcher) dispatchOnce(ctx context.Context) {
	var watchers []database.Watcher
	if err := d.db.Where("webhook_enabled = ?", true).Find(&watchers).Error; err != nil {
		d.log.Warn("webhook dispatcher: list watchers failed", "error", err)
		return
	}
	var wg sync.WaitGroup
	for i := range watchers {
		wg.Add(1)
		watcher := watchers[i]
		go func() {
			defer wg.Done()
			d.dispatchWatcher(ctx, &watcher)
		}()
	}
	wg.Wait()
}

func (d *Dispatcher) dispatchWatcher(ctx context.Context, watcher *database.Watcher) {
	resolved := ResolveConfig(d.appCfg, watcher)
	if !resolved.Enabled || strings.TrimSpace(resolved.URL) == "" {
		return
	}
	if strings.TrimSpace(resolved.SigningSecret) == "" {
		return
	}

	var event database.WebhookEvent
	if err := d.db.Where("watcher_id = ? AND status = ?", watcher.ID, EventStatusPending).
		Order("occurred_at asc, id asc").First(&event).Error; err != nil {
		return
	}

	var last database.WebhookDelivery
	err := d.db.Where("watcher_id = ? AND webhook_event_id = ?", watcher.ID, event.ID).Order("attempt_number desc").First(&last).Error
	if err == nil && last.Status == DeliveryStatusRetryWait && last.NextRetryAt != nil && last.NextRetryAt.After(time.Now().UTC()) {
		return
	}

	attemptNum := 1
	if err == nil {
		attemptNum = last.AttemptNumber + 1
	}
	delivery := database.WebhookDelivery{
		WatcherID:      watcher.ID,
		WebhookEventID: event.ID,
		DeliveryID:     fmt.Sprintf("delv_%s", strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")),
		Status:         DeliveryStatusPending,
		AttemptNumber:  attemptNum,
		ResolvedURL:    resolved.URL,
		AuthType:       "standard_webhooks_hmac_sha256",
		SecretSource:   resolved.SecretSource,
	}
	if err := d.db.Create(&delivery).Error; err != nil {
		d.log.Warn("webhook dispatcher: create delivery failed", "error", err)
		return
	}

	reqCtx, cancel := context.WithTimeout(ctx, resolved.Timeout)
	defer cancel()
	now := time.Now().UTC()
	delivery.LastAttemptAt = &now
	statusCode, respBody, signature, sendErr := d.send(reqCtx, resolved, &event, &delivery, now)

	update := map[string]any{
		"last_attempt_at":      &now,
		"response_status_code": statusCode,
		"response_body":        respBody,
		"webhook_timestamp":    now.Unix(),
		"webhook_signature":    signature,
	}
	if sendErr == nil && statusCode >= 200 && statusCode < 300 {
		update["status"] = DeliveryStatusSucceeded
		update["completed_at"] = &now
		_ = d.db.Model(&delivery).Updates(update).Error
		_ = d.db.Model(&event).Updates(map[string]any{
			"status":           EventStatusDelivered,
			"last_delivery_at": &now,
		}).Error
		_ = d.db.Model(watcher).Update("webhook_failure_streak", 0).Error
		return
	}

	errText := ""
	if sendErr != nil {
		errText = sendErr.Error()
	} else if respBody != "" {
		errText = respBody
	} else {
		errText = fmt.Sprintf("unexpected HTTP %d", statusCode)
	}

	retryable := isRetryable(statusCode, sendErr)
	update["error"] = errText

	var streak int
	_ = d.db.Model(watcher).Select("webhook_failure_streak").First(watcher, watcher.ID).Error
	streak = watcher.WebhookFailureStreak + 1
	_ = d.db.Model(watcher).Update("webhook_failure_streak", streak).Error

	if retryable && attemptNum < len(resolved.RetrySchedule) {
		nextRetry := now.Add(resolved.RetrySchedule[attemptNum])
		update["status"] = DeliveryStatusRetryWait
		update["next_retry_at"] = &nextRetry
		_ = d.db.Model(&delivery).Updates(update).Error
		return
	}

	update["status"] = DeliveryStatusFailed
	update["completed_at"] = &now
	_ = d.db.Model(&delivery).Updates(update).Error
	_ = d.db.Model(&event).Updates(map[string]any{
		"status":           EventStatusExhausted,
		"last_delivery_at": &now,
	}).Error

	if resolved.AutoPauseEnabled && streak >= resolved.AutoPauseAfter {
		pausedAt := time.Now().UTC()
		_ = d.db.Model(watcher).Updates(map[string]any{
			"webhook_paused_at":    &pausedAt,
			"webhook_pause_reason": fmt.Sprintf("auto-paused after %d consecutive delivery failures", streak),
		}).Error
	}

	_ = d.db.Transaction(func(tx *gorm.DB) error {
		return d.service.EmitDeliveryExhausted(tx, watcher, &event, &delivery)
	})
}

func (d *Dispatcher) send(ctx context.Context, cfg ResolvedConfig, event *database.WebhookEvent, delivery *database.WebhookDelivery, timestamp time.Time) (int, string, string, error) {
	wh, err := NewStandardWebhook(cfg.SigningSecret)
	if err != nil {
		return 0, "", "", err
	}
	signature, err := wh.Sign(event.EventID, timestamp, []byte(event.Payload))
	if err != nil {
		return 0, "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewBufferString(event.Payload))
	if err != nil {
		return 0, "", signature, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Watcher-Event", event.EventType)
	req.Header.Set("X-Watcher-Delivery-ID", delivery.DeliveryID)
	req.Header.Set(HeaderWebhookID, event.EventID)
	req.Header.Set(HeaderWebhookTimestamp, fmt.Sprintf("%d", timestamp.Unix()))
	req.Header.Set(HeaderWebhookSignature, signature)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", signature, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
	return resp.StatusCode, string(body), signature, nil
}

func isRetryable(status int, err error) bool {
	if err != nil {
		if errors.Is(err, ErrInvalidSigningSecret) {
			return false
		}
		return true
	}
	if status == http.StatusTooManyRequests {
		return true
	}
	return status >= 500
}

func (d *Dispatcher) cleanupExpired() {
	if d.appCfg == nil {
		return
	}
	eventCutoff := time.Now().UTC().AddDate(0, 0, -max(d.appCfg.WebhookEventRetentionDays, 1))
	deliveryCutoff := time.Now().UTC().AddDate(0, 0, -max(d.appCfg.WebhookDeliveryRetentionDays, 1))
	_ = d.db.Where("created_at < ?", deliveryCutoff).Delete(&database.WebhookDelivery{}).Error
	_ = d.db.Where("created_at < ?", eventCutoff).Delete(&database.WebhookEvent{}).Error
}
