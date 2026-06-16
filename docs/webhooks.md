# Webhooks

Watcher can deliver outbound webhook events for watcher lifecycle changes, rollback outcomes, and service health changes.

This document is the public contract for webhook behavior in Watcher `schema_version: "v1"`.

## Delivery Model

- Webhooks are optional and configured per watcher.
- A watcher may use its own webhook URL and bearer token override, or inherit global defaults from `/api/self/config`.
- Delivery is **at-least-once**. Receivers should deduplicate using `event_id`.
- Each HTTP attempt gets its own `delivery_id`.
- Delivery order is FIFO per watcher. A later event waits behind the oldest pending or retry-waiting event for that watcher.
- Any `2xx` response counts as success.
- Retryable failures:
  - network errors
  - timeouts
  - `429`
  - `5xx`
- Non-retryable final failures:
  - other `4xx` responses
- If auto-pause is enabled and the watcher reaches the configured consecutive failure threshold, outbound delivery is paused. New events are stored as `suppressed` until an operator resumes delivery.
- Resuming webhook delivery can optionally replay suppressed events in original order.
- Replay keeps the original event payload and `occurred_at`, but creates a new `delivery_id`.

## HTTP Request

Watcher sends:

- Method: `POST`
- Content-Type: `application/json`
- Authorization: `Bearer <token>` when a bearer token is configured
- `X-Watcher-Event: <event_type>`
- `X-Watcher-Delivery-ID: <delivery_id>`

## Common Payload Envelope

Every event payload includes:

```json
{
  "schema_version": "v1",
  "event_id": "evt_or_uuid",
  "event_type": "watcher.deployment_failed",
  "occurred_at": "2026-06-16T10:00:00Z",
  "watcher": {
    "id": 12,
    "name": "api-prod"
  },
  "summary": "Deployment of api-prod to 1.4.2 failed during health_check"
}
```

Depending on the event type, additional nested objects such as `version`, `attempt`, `service`, `health`, or `failed_delivery` are included.

## Event Catalog

### `watcher.version_found`

When it fires:
- When polling discovers a newer remote version than the watcher `current_version`.

Behavior:
- Emitted once per watcher per discovered version.
- Repeated polls for the same version are deduplicated.
- Can fire even when the watcher will not deploy yet.
- This is a discovery event only and is not linked to a deploy attempt row.

Payload fields:
- `version.discovered_version`
- `version.current_version`
- `version.will_deploy`
- `version.block_reason`

### `watcher.deployment_succeeded`

When it fires:
- When a deploy attempt reaches terminal success.

Behavior:
- Emitted from the deploy attempt record.
- Includes the attempt reason and initiator so manual redeploys can be distinguished from agent-driven rollout.

Payload fields:
- `attempt.id`
- `attempt.kind = "deploy"`
- `attempt.reason`
- `attempt.triggered_by`
- `attempt.status = "succeeded"`
- `attempt.target_version`
- `attempt.from_version`

### `watcher.deployment_failed`

When it fires:
- When a deploy attempt reaches terminal failure.

Behavior:
- Emitted as soon as the deploy attempt is marked failed.
- If automatic rollback later runs, that rollback is represented by its own event.
- `failure_phase` identifies where the deploy failed.

Payload fields:
- `attempt.id`
- `attempt.kind = "deploy"`
- `attempt.reason`
- `attempt.triggered_by`
- `attempt.status = "failed"`
- `attempt.target_version`
- `attempt.from_version`
- `attempt.failure_phase`
- `attempt.error`
- `attempt.parent_attempt_id`
- `attempt.root_attempt_id`

### `watcher.rollback_succeeded`

When it fires:
- When a rollback attempt completes successfully.

Behavior:
- Emitted for both automatic and manual rollback attempts.
- Includes both the restored version and the failed target version that led to the rollback.
- The watcher live status returns to `healthy`, but the attempt history still records the rollback attempt.

Payload fields:
- `attempt.id`
- `attempt.kind = "rollback"`
- `attempt.reason`
- `attempt.triggered_by`
- `attempt.status = "succeeded"`
- `attempt.target_version`
- `attempt.failed_target_version`
- `attempt.parent_attempt_id`
- `attempt.root_attempt_id`

### `watcher.rollback_failed`

When it fires:
- When a rollback attempt itself fails.

Behavior:
- Separate from `watcher.deployment_failed`.
- Use parent/root attempt linkage to correlate the rollback failure back to the deploy incident.

Payload fields:
- `attempt.id`
- `attempt.kind = "rollback"`
- `attempt.reason`
- `attempt.triggered_by`
- `attempt.status = "failed"`
- `attempt.target_version`
- `attempt.failed_target_version`
- `attempt.error`
- `attempt.parent_attempt_id`
- `attempt.root_attempt_id`

### `service.health_changed`

When it fires:
- When the stored health state for a service changes.

Behavior:
- Only emitted on state changes. Repeated checks with the same state are suppressed.
- Today, Watcher emits this from manual service health checks through `GET /api/services/:id/health`.
- The payload reserves `health.source` for future `deploy`, `rollback`, and `monitor` emitters.
- This event is service-scoped; watcher context is included, but the primary subject is the service.

Payload fields:
- `service.id`
- `service.name`
- `service.service_type`
- `service.health_check_url`
- `health.previous_status`
- `health.current_status`
- `health.http_status`
- `health.error`
- `health.checked_at`
- `health.source`

### `watcher.webhook_test`

When it fires:
- When an operator triggers `Send Test Webhook`.

Behavior:
- Uses the same outbox, retry, pause, and delivery history pipeline as real webhook events.
- Not gated by the business-event subscription booleans.

Payload fields:
- `triggered_by = "manual"`

### `webhook.delivery_exhausted`

When it fires:
- After a previously emitted event finishes its final retry and still fails delivery.

Behavior:
- Emitted for exhausted attempted deliveries only.
- Not emitted for events that were only suppressed while delivery was paused.
- Non-recursive: if `webhook.delivery_exhausted` itself cannot be delivered, Watcher records that internally and does not emit another exhaustion event.

Payload fields:
- `failed_delivery.event_id`
- `failed_delivery.event_type`
- `failed_delivery.delivery_id`
- `failed_delivery.attempt_number`
- `failed_delivery.response_status_code`
- `failed_delivery.error`
- `failed_delivery.summary`

## Event Status vs Delivery Status

Webhook event rows use:
- `pending`
- `delivered`
- `suppressed`
- `exhausted`

Delivery attempt rows use:
- `pending`
- `retry_wait`
- `succeeded`
- `failed`

## Watcher API Endpoints

- `GET /api/watchers/:id/webhook-events`
- `GET /api/watchers/:id/webhook-deliveries`
- `GET /api/watchers/:id/webhook-deliveries/:deliveryId`
- `POST /api/watchers/:id/webhook/test`
- `POST /api/watchers/:id/webhook/resume`

