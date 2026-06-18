package webhook

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Service struct {
	db      *gorm.DB
	appCfg  *config.AppConfig
	trigger chan struct{}
}

func NewService(db *gorm.DB, appCfg *config.AppConfig, trigger chan struct{}) *Service {
	return &Service{db: db, appCfg: appCfg, trigger: trigger}
}

func (s *Service) Nudge() {
	if s == nil || s.trigger == nil {
		return
	}
	select {
	case s.trigger <- struct{}{}:
	default:
	}
}

func (s *Service) EmitTestEvent(watcher *database.Watcher) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		payload := buildEventPayload(EventWebhookTest, watcher, fmt.Sprintf("Webhook test for watcher %s", watcher.Name), map[string]any{
			"triggered_by": "manual",
		})
		_, err := s.insertEvent(tx, watcher, EventWebhookTest, "", payload, "Webhook test event")
		return err
	})
}

func (s *Service) EmitVersionFoundTx(tx *gorm.DB, watcher *database.Watcher, discoveredVersion, currentVersion string, willDeploy bool, blockReason string) error {
	_, err := s.insertEvent(tx, watcher, EventVersionFound, fmt.Sprintf("version_found:%d:%s", watcher.ID, discoveredVersion), buildEventPayload(EventVersionFound, watcher, fmt.Sprintf("Watcher %s found version %s", watcher.Name, discoveredVersion), map[string]any{
		"version": map[string]any{
			"discovered_version": discoveredVersion,
			"current_version":    currentVersion,
			"will_deploy":        willDeploy,
			"block_reason":       blockReason,
		},
	}), fmt.Sprintf("Watcher %s found version %s", watcher.Name, discoveredVersion))
	return err
}

func (s *Service) EmitAttemptEventTx(tx *gorm.DB, watcher *database.Watcher, attempt *database.DeployLog) error {
	if attempt == nil {
		return nil
	}
	eventType := ""
	switch attempt.Kind {
	case "deploy":
		if attempt.Status == "succeeded" {
			eventType = EventDeploymentSucceeded
		} else if attempt.Status == "failed" {
			eventType = EventDeploymentFailed
		}
	case "rollback":
		if attempt.Status == "succeeded" {
			eventType = EventRollbackSucceeded
		} else if attempt.Status == "failed" {
			eventType = EventRollbackFailed
		}
	}
	if eventType == "" {
		return nil
	}

	summary := buildAttemptSummary(watcher, attempt, eventType)
	payload := buildEventPayload(eventType, watcher, summary, map[string]any{
		"attempt": map[string]any{
			"id":                    attempt.ID,
			"kind":                  attempt.Kind,
			"reason":                attempt.Reason,
			"status":                attempt.Status,
			"triggered_by":          attempt.TriggeredBy,
			"target_version":        attempt.Version,
			"from_version":          attempt.FromVersion,
			"failed_target_version": attempt.FailedTargetVersion,
			"failure_phase":         attempt.FailurePhase,
			"error":                 attempt.Error,
			"parent_attempt_id":     attempt.ParentAttemptID,
			"root_attempt_id":       attempt.RootAttemptID,
		},
	})
	_, err := s.insertEvent(tx, watcher, eventType, "", payload, summary)
	return err
}

func (s *Service) EmitHealthChangedTx(tx *gorm.DB, watcher *database.Watcher, svc *database.Service, event *database.HealthEvent) error {
	if event == nil || event.PreviousStatus == event.Status {
		return nil
	}
	summary := fmt.Sprintf("Service %s health changed from %s to %s", svc.WindowsServiceName, valueOrUnknown(event.PreviousStatus), event.Status)
	payload := buildEventPayload(EventServiceHealthChanged, watcher, summary, map[string]any{
		"service": map[string]any{
			"id":               svc.ID,
			"name":             svc.WindowsServiceName,
			"service_type":     svc.ServiceType,
			"health_check_url": svc.HealthCheckURL,
		},
		"health": map[string]any{
			"previous_status": event.PreviousStatus,
			"current_status":  event.Status,
			"http_status":     event.HTTPStatus,
			"error":           event.Error,
			"checked_at":      event.CheckedAt,
			"source":          event.Source,
		},
	})
	_, err := s.insertEvent(tx, watcher, EventServiceHealthChanged, "", payload, summary)
	return err
}

func (s *Service) EmitDeliveryExhausted(tx *gorm.DB, watcher *database.Watcher, ev *database.WebhookEvent, delivery *database.WebhookDelivery) error {
	if ev == nil || delivery == nil || ev.EventType == EventDeliveryExhausted {
		return nil
	}
	payload := buildEventPayload(EventDeliveryExhausted, watcher, fmt.Sprintf("Webhook delivery exhausted for %s", ev.EventType), map[string]any{
		"failed_delivery": map[string]any{
			"event_id":             ev.EventID,
			"event_type":           ev.EventType,
			"delivery_id":          delivery.DeliveryID,
			"attempt_number":       delivery.AttemptNumber,
			"response_status_code": delivery.ResponseStatusCode,
			"error":                delivery.Error,
			"summary":              ev.Summary,
		},
	})
	_, err := s.insertEvent(tx, watcher, EventDeliveryExhausted, fmt.Sprintf("delivery_exhausted:%s", ev.EventID), payload, fmt.Sprintf("Webhook delivery exhausted for %s", ev.EventType))
	return err
}

func (s *Service) insertEvent(tx *gorm.DB, watcher *database.Watcher, eventType, dedupeKey string, payload map[string]any, summary string) (*database.WebhookEvent, error) {
	if watcher == nil || tx == nil {
		return nil, nil
	}
	if !watcherShouldReceive(watcher, eventType) {
		return nil, nil
	}

	if dedupeKey != "" {
		var existing database.WebhookEvent
		if err := tx.Where("watcher_id = ? AND event_type = ? AND dedupe_key = ?", watcher.ID, eventType, dedupeKey).First(&existing).Error; err == nil {
			return &existing, nil
		}
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	resolved := ResolveConfig(s.appCfg, watcher)
	status := EventStatusPending
	var suppressedAt *time.Time
	if watcher.WebhookPausedAt != nil {
		now := time.Now().UTC()
		suppressedAt = &now
		status = EventStatusSuppressed
	}
	if !resolved.Enabled || strings.TrimSpace(resolved.URL) == "" || strings.TrimSpace(resolved.SigningSecret) == "" {
		return nil, nil
	}

	event := &database.WebhookEvent{
		WatcherID:     watcher.ID,
		EventID:       payload["event_id"].(string),
		SchemaVersion: SchemaVersionV1,
		EventType:     eventType,
		DedupeKey:     dedupeKey,
		Status:        status,
		Summary:       summary,
		Payload:       string(raw),
		OccurredAt:    time.Now().UTC(),
		SuppressedAt:  suppressedAt,
	}
	if err := tx.Create(event).Error; err != nil {
		return nil, err
	}
	if status == EventStatusPending {
		s.Nudge()
	}
	return event, nil
}

func buildEventPayload(eventType string, watcher *database.Watcher, summary string, extra map[string]any) map[string]any {
	eventID := uuid.NewString()
	occurredAt := time.Now().UTC().Format(time.RFC3339)
	watcherPayload := map[string]any{
		"id":   watcher.ID,
		"name": watcher.Name,
	}
	data := map[string]any{
		"schema_version": SchemaVersionV1,
		"event_id":       eventID,
		"watcher":        watcherPayload,
		"summary":        summary,
	}
	for key, value := range extra {
		data[key] = value
	}

	payload := map[string]any{
		"schema_version": SchemaVersionV1,
		"event_id":       eventID,
		"event_type":     eventType,
		"type":           eventType,
		"occurred_at":    occurredAt,
		"timestamp":      occurredAt,
		"watcher":        watcherPayload,
		"data":           data,
		"summary":        summary,
	}
	for key, value := range extra {
		payload[key] = value
	}
	return payload
}

func watcherShouldReceive(w *database.Watcher, eventType string) bool {
	if w == nil || !w.WebhookEnabled {
		return false
	}
	switch eventType {
	case EventVersionFound:
		return w.NotifyVersionFound
	case EventDeploymentSucceeded:
		return w.NotifyDeploymentSucceeded
	case EventDeploymentFailed:
		return w.NotifyDeploymentFailed
	case EventRollbackSucceeded:
		return w.NotifyRollbackSucceeded
	case EventRollbackFailed:
		return w.NotifyRollbackFailed
	case EventServiceHealthChanged:
		return w.NotifyServiceHealthChanged
	case EventWebhookTest, EventDeliveryExhausted:
		return true
	default:
		return false
	}
}

func buildAttemptSummary(watcher *database.Watcher, attempt *database.DeployLog, eventType string) string {
	switch eventType {
	case EventDeploymentSucceeded:
		return fmt.Sprintf("Deployment of %s to %s succeeded", watcher.Name, attempt.Version)
	case EventDeploymentFailed:
		if attempt.FailurePhase != "" {
			return fmt.Sprintf("Deployment of %s to %s failed during %s", watcher.Name, attempt.Version, attempt.FailurePhase)
		}
		return fmt.Sprintf("Deployment of %s to %s failed", watcher.Name, attempt.Version)
	case EventRollbackSucceeded:
		return fmt.Sprintf("Rollback of %s restored %s after %s failed", watcher.Name, attempt.Version, valueOrUnknown(attempt.FailedTargetVersion))
	case EventRollbackFailed:
		return fmt.Sprintf("Rollback of %s to %s failed", watcher.Name, attempt.Version)
	default:
		return fmt.Sprintf("%s %s", attempt.Kind, attempt.Status)
	}
}

func valueOrUnknown(v string) string {
	if strings.TrimSpace(v) == "" {
		return "unknown"
	}
	return v
}
