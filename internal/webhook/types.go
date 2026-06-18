package webhook

import "time"

const (
	SchemaVersionV1 = "v1"

	EventVersionFound         = "watcher.version_found"
	EventDeploymentSucceeded  = "watcher.deployment_succeeded"
	EventDeploymentFailed     = "watcher.deployment_failed"
	EventRollbackSucceeded    = "watcher.rollback_succeeded"
	EventRollbackFailed       = "watcher.rollback_failed"
	EventServiceHealthChanged = "service.health_changed"
	EventWebhookTest          = "watcher.webhook_test"
	EventDeliveryExhausted    = "webhook.delivery_exhausted"

	EventStatusPending    = "pending"
	EventStatusDelivered  = "delivered"
	EventStatusSuppressed = "suppressed"
	EventStatusExhausted  = "exhausted"

	DeliveryStatusPending   = "pending"
	DeliveryStatusRetryWait = "retry_wait"
	DeliveryStatusSucceeded = "succeeded"
	DeliveryStatusFailed    = "failed"
)

type ResolvedConfig struct {
	Enabled               bool
	URL                   string
	SigningSecret         string
	SecretSource          string
	Timeout               time.Duration
	RetrySchedule         []time.Duration
	AutoPauseEnabled      bool
	AutoPauseAfter        int
	EventRetentionDays    int
	DeliveryRetentionDays int
}
