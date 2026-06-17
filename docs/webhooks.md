# Watcher Outbound Webhooks Integration Guide

Watcher can emit durable, outbound HTTP webhook events to external systems to announce watcher lifecycle updates, rollout completions, rollback statuses, and service health changes. 

This document serves as the integration guide and public API contract for webhook interactions in Watcher (`schema_version: "v1"`).

---

## 1. Webhook Delivery Model & Guarantees

Watcher's webhook engine is built with reliability in mind, providing the following guarantees and behaviors:

| Attribute | Guarantee & Behavior |
| :--- | :--- |
| **Delivery Mechanism** | Outbound `POST` requests sending `application/json` payload bodies. |
| **Delivery Guarantee** | **At-least-once** delivery. Due to network failures or retries, your endpoint may receive the same event multiple times. Receivers **must** deduplicate processing using the unique `event_id` field. |
| **Delivery Ordering** | **FIFO (First-In, First-Out) per Watcher**. Webhook events are processed sequentially in the order they were queued. If an event fails and is scheduled for a retry, subsequent events for that same watcher will wait behind it to preserve temporal order. |
| **Success Criteria** | Any HTTP response code in the `2xx` range (e.g., `200 OK`, `202 Accepted`) is treated as a successful delivery. |
| **Retry Behavior** | Network errors, timeouts, `429 Too Many Requests`, and server errors (`5xx`) trigger automatic retries. Retries are scheduled according to the `webhook_retry_schedule_sec` settings. |
| **Non-Retryable Errors** | Client errors in the `4xx` range (except `429`), such as `400 Bad Request`, `401 Unauthorized`, or `404 Not Found`, are classified as final failures. The event is moved directly to the `exhausted` state. |
| **Auto-Pause Circuit Breaker** | If enabled (`webhook_auto_pause_enabled`), reaching the threshold of consecutive failures will automatically pause outbound delivery for that watcher. New events generated while paused are kept as `suppressed` in the outbox. |
| **Manual Resume / Replays** | Resuming a paused outbox gives operators the option to replay suppressed events in their original chronological order, generating new `delivery_id` headers but preserving the original `event_id` and payload. |

---

## 2. HTTP Request Structure

When delivering an event, Watcher sends an HTTP request with the following details:

- **Method**: `POST`
- **Content-Type**: `application/json`
- **Headers**:
  - `X-Watcher-Event`: The type of event being delivered (e.g. `watcher.deployment_succeeded`).
  - `X-Watcher-Delivery-ID`: A unique string identifying this specific delivery attempt (e.g. `dlv_01j0deployok_1`).
  - `Authorization`: `Bearer <token>` (only included if a Bearer Token is configured for the watcher or globally).

---

## 3. Webhook Receiver Examples

To integrate with Watcher, write an HTTP server endpoint that checks the `Authorization` header, logs/processes the payload asynchronously, and returns a `200 OK` or `202 Accepted` immediately.

### Go Example (Using Standard Library)

```go
package main

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"log"
	"net/http"
)

const expectedToken = "your-configured-bearer-token"

func webhookHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Verify Request Method
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 2. Authenticate using Bearer Token
	authHeader := r.Header.Get("Authorization")
	expectedAuth := "Bearer " + expectedToken
	// Use ConstantTimeCompare to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(authHeader), []byte(expectedAuth)) != 1 {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 3. Extract Watcher Custom Headers
	eventType := r.Header.Get("X-Watcher-Event")
	deliveryID := r.Header.Get("X-Watcher-Delivery-ID")

	// 4. Read Body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// 5. Parse JSON Payload Envelope
	var envelope struct {
		EventID   string `json:"event_id"`
		EventType string `json:"event_type"`
		Summary   string `json:"summary"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// 6. Log metadata and handoff to a background queue
	log.Printf("Received Webhook: ID=%s EventType=%s (%s) DeliveryID=%s", 
		envelope.EventID, eventType, envelope.Summary, deliveryID)

	// TODO: Store eventID in database to deduplicate future deliveries
	// Go routine or message queue handoff here to process actual payload...

	// 7. Respond with 200 OK or 202 Accepted
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"received"}`))
}

func main() {
	http.HandleFunc("/watcher-webhooks", webhookHandler)
	log.Println("Webhook receiver running on :8090...")
	if err := http.ListenAndServe(":8090", nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
```

### Node.js Example (Express + TypeScript)

```typescript
import express, { Request, Response } from 'express';

const app = express();
app.use(express.json());

const EXPECTED_TOKEN = 'your-configured-bearer-token';

interface WebhookEnvelope {
  schema_version: string;
  event_id: string;
  event_type: string;
  occurred_at: string;
  watcher: {
    id: number;
    name: string;
  };
  summary: string;
}

app.post('/watcher-webhooks', (req: Request, res: Response): any => {
  const authHeader = req.headers.authorization;
  const eventType = req.headers['x-watcher-event'];
  const deliveryId = req.headers['x-watcher-delivery-id'];

  // 1. Authenticate Token
  if (!authHeader || authHeader !== `Bearer ${EXPECTED_TOKEN}`) {
    return res.status(401).send('Unauthorized');
  }

  const payload = req.body as WebhookEnvelope;

  // 2. Validate basic structure
  if (!payload.event_id || !payload.event_type) {
    return res.status(400).send('Bad Request: Missing event_id or event_type');
  }

  console.log(`Processing event ${payload.event_id} of type ${eventType}. Attempt delivery: ${deliveryId}`);
  console.log(`Summary: ${payload.summary}`);

  // TODO: Check if event_id has been processed recently (deduplication)
  // Process the webhook payload asynchronously (e.g., notify Slack, update status boards)

  // 3. Return Success Status Code
  res.status(200).json({ status: 'accepted' });
});

app.listen(8090, () => {
  console.log('Webhook server listening on port 8090');
});
```

---

## 4. OpenAPI Specification Component Schemas

Below is the OpenAPI 3.0 YAML definition of the payload models. Use these schemas in your API gateway or mock servers to validate incoming webhook formats.

```yaml
components:
  schemas:
    BaseWebhookEvent:
      type: object
      required:
        - schema_version
        - event_id
        - event_type
        - occurred_at
        - watcher
        - summary
      properties:
        schema_version:
          type: string
          example: "v1"
          description: "Version of the webhook payload schema structure."
        event_id:
          type: string
          example: "evt_01j0versionfound"
          description: "Unique identifier for this event. Use this value to deduplicate event handling."
        event_type:
          type: string
          example: "watcher.version_found"
          description: "Matches the X-Watcher-Event header."
        occurred_at:
          type: string
          format: date-time
          example: "2026-06-17T02:15:10Z"
          description: "RFC3339 formatted timestamp representing when the event was generated."
        watcher:
          type: object
          required:
            - id
            - name
          properties:
            id:
              type: integer
              example: 12
              description: "Database ID of the originating RepoWatcher."
            name:
              type: string
              example: "api-prod"
              description: "Display name of the watcher."
        summary:
          type: string
          example: "Watcher api-prod found version v1.4.2"
          description: "Human-readable summary sentence describing the event."

    DeployAttempt:
      type: object
      required:
        - id
        - kind
        - reason
        - status
        - triggered_by
        - target_version
        - from_version
      properties:
        id:
          type: integer
          example: 301
          description: "ID of the deploy/rollback transaction log in Watcher."
        kind:
          type: string
          enum: [deploy, rollback]
          example: "deploy"
          description: "Determines whether this was a deployment or recovery rollback attempt."
        reason:
          type: string
          example: "new_version_found"
          description: "Indicates trigger source, e.g. new_version_found, manual_redeploy, auto_after_failed_deploy, manual_rollback."
        status:
          type: string
          enum: [succeeded, failed]
          example: "succeeded"
          description: "Terminal state of the attempt transaction."
        triggered_by:
          type: string
          enum: [agent, manual]
          example: "agent"
          description: "Who initiated the execution."
        target_version:
          type: string
          example: "v1.4.2"
          description: "Target version string of the release."
        from_version:
          type: string
          example: "v1.4.1"
          description: "Previous active version before starting the transaction."
        failed_target_version:
          type: string
          example: ""
          description: "Only populated for rollbacks: the version that failed and triggered the rollback."
        failure_phase:
          type: string
          example: ""
          description: "Identifies step of failure, e.g. download, extract, activate_release, start_services, health_check."
        error:
          type: string
          example: ""
          description: "Failure reason or terminal process error message."
        parent_attempt_id:
          type: integer
          nullable: true
          example: null
          description: "Link to deployment event that triggered an automatic rollback attempt."
        root_attempt_id:
          type: integer
          example: 301
          description: "Identifies the parent transaction chain ID."

    WatcherVersionFoundEvent:
      allOf:
        - $ref: '#/components/schemas/BaseWebhookEvent'
        - type: object
          required:
            - version
          properties:
            version:
              type: object
              required:
                - discovered_version
                - current_version
                - will_deploy
                - block_reason
              properties:
                discovered_version:
                  type: string
                  example: "v1.4.2"
                current_version:
                  type: string
                  example: "v1.4.1"
                will_deploy:
                  type: boolean
                  example: false
                block_reason:
                  type: string
                  example: "watcher paused"

    WatcherDeploymentSucceededEvent:
      allOf:
        - $ref: '#/components/schemas/BaseWebhookEvent'
        - type: object
          required:
            - attempt
          properties:
            attempt:
              $ref: '#/components/schemas/DeployAttempt'

    WatcherDeploymentFailedEvent:
      allOf:
        - $ref: '#/components/schemas/BaseWebhookEvent'
        - type: object
          required:
            - attempt
          properties:
            attempt:
              $ref: '#/components/schemas/DeployAttempt'

    WatcherRollbackSucceededEvent:
      allOf:
        - $ref: '#/components/schemas/BaseWebhookEvent'
        - type: object
          required:
            - attempt
          properties:
            attempt:
              $ref: '#/components/schemas/DeployAttempt'

    WatcherRollbackFailedEvent:
      allOf:
        - $ref: '#/components/schemas/BaseWebhookEvent'
        - type: object
          required:
            - attempt
          properties:
            attempt:
              $ref: '#/components/schemas/DeployAttempt'

    ServiceHealthChangedEvent:
      allOf:
        - $ref: '#/components/schemas/BaseWebhookEvent'
        - type: object
          required:
            - service
            - health
          properties:
            service:
              type: object
              required:
                - id
                - name
                - service_type
                - health_check_url
              properties:
                id:
                  type: integer
                  example: 87
                name:
                  type: string
                  example: "api-prod-web"
                service_type:
                  type: string
                  example: "nssm"
                health_check_url:
                  type: string
                  example: "https://api.example.com/health"
            health:
              type: object
              required:
                - previous_status
                - current_status
                - http_status
                - error
                - checked_at
                - source
              properties:
                previous_status:
                  type: string
                  example: "healthy"
                current_status:
                  type: string
                  example: "unhealthy"
                http_status:
                  type: integer
                  example: 503
                error:
                  type: string
                  example: "unexpected status code 503"
                checked_at:
                  type: string
                  format: date-time
                  example: "2026-06-17T03:02:11Z"
                source:
                  type: string
                  example: "manual"

    WatcherWebhookTestEvent:
      allOf:
        - $ref: '#/components/schemas/BaseWebhookEvent'
        - type: object
          required:
            - triggered_by
          properties:
            triggered_by:
              type: string
              example: "manual"

    WebhookDeliveryExhaustedEvent:
      allOf:
        - $ref: '#/components/schemas/BaseWebhookEvent'
        - type: object
          required:
            - failed_delivery
          properties:
            failed_delivery:
              type: object
              required:
                - event_id
                - event_type
                - delivery_id
                - attempt_number
                - response_status_code
                - error
                - summary
              properties:
                event_id:
                  type: string
                  example: "evt_01j0deployfail"
                event_type:
                  type: string
                  example: "watcher.deployment_failed"
                delivery_id:
                  type: string
                  example: "dlv_01j0deployfail_4"
                attempt_number:
                  type: integer
                  example: 4
                response_status_code:
                  type: integer
                  example: 503
                error:
                  type: string
                  example: "server returned 503"
                summary:
                  type: string
                  example: "Deployment of api-prod to v1.4.3 failed during health_check"
```

---

## 5. Event Catalog & Payload Reference

This section details every event type fired by Watcher. Each block defines the trigger conditions, behaviors, and provides a matching JSON payload example.

### `watcher.version_found`

* **Trigger**: Fired when a polling cycle discovers a newer remote release tag than the watcher's current active version.
* **Behavior**:
  - Emitted exactly once per remote version discovered.
  - Dedicated deduplication is performed so subsequent poll checks for the same version do not repeat the event.
  - This event fires even if deployment is currently blocked (e.g., watcher is paused, release ref mismatch, or retry threshold exceeded). Check the nested `version.will_deploy` and `version.block_reason` fields to see if the watcher will proceed to roll out this version.
* **Payload Structure**:
```json
{
  "schema_version": "v1",
  "event_id": "evt_01j0versionfound",
  "event_type": "watcher.version_found",
  "occurred_at": "2026-06-17T02:15:10Z",
  "watcher": {
    "id": 12,
    "name": "api-prod"
  },
  "version": {
    "discovered_version": "v1.4.2",
    "current_version": "v1.4.1",
    "will_deploy": false,
    "block_reason": "watcher paused"
  },
  "summary": "Watcher api-prod found version v1.4.2"
}
```

---

### `watcher.deployment_succeeded`

* **Trigger**: Fired when a deployment attempt finishes successfully (i.e. artifact is extracted, services are swapped, and health checks pass).
* **Behavior**:
  - Emitted only after the active version is updated in the database.
  - Useful for updating deployment dashboards or triggering post-deployment automation (e.g. cache purges).
* **Payload Structure**:
```json
{
  "schema_version": "v1",
  "event_id": "evt_01j0deployok",
  "event_type": "watcher.deployment_succeeded",
  "occurred_at": "2026-06-17T02:20:03Z",
  "watcher": {
    "id": 12,
    "name": "api-prod"
  },
  "attempt": {
    "id": 301,
    "kind": "deploy",
    "reason": "new_version_found",
    "status": "succeeded",
    "triggered_by": "agent",
    "target_version": "v1.4.2",
    "from_version": "v1.4.1",
    "failed_target_version": "",
    "failure_phase": "",
    "error": "",
    "parent_attempt_id": null,
    "root_attempt_id": 301
  },
  "summary": "Deployment of api-prod to v1.4.2 succeeded"
}
```

---

### `watcher.deployment_failed`

* **Trigger**: Fired when a deployment attempt hits an unrecoverable failure or times out during installation or health checks.
* **Behavior**:
  - Fired *immediately* when the failure occurs, before any automatic recovery rollback is initiated.
  - The `failure_phase` field informs you of where the sequence broke down (e.g., `download` vs `health_check`).
* **Payload Structure**:
```json
{
  "schema_version": "v1",
  "event_id": "evt_01j0deployfail",
  "event_type": "watcher.deployment_failed",
  "occurred_at": "2026-06-17T02:21:17Z",
  "watcher": {
    "id": 12,
    "name": "api-prod"
  },
  "attempt": {
    "id": 302,
    "kind": "deploy",
    "reason": "new_version_found",
    "status": "failed",
    "triggered_by": "agent",
    "target_version": "v1.4.3",
    "from_version": "v1.4.2",
    "failed_target_version": "",
    "failure_phase": "health_check",
    "error": "health check returned 503",
    "parent_attempt_id": null,
    "root_attempt_id": 302
  },
  "summary": "Deployment of api-prod to v1.4.3 failed during health_check"
}
```

---

### `watcher.rollback_succeeded`

* **Trigger**: Fired when a recovery rollback attempt successfully finishes.
* **Behavior**:
  - Triggered for both automated agent rollbacks (after a failed deploy) and manual operator rollbacks.
  - The `failed_target_version` identifies which release version caused the problem, while the `target_version` indicates the safe backup version that was successfully restored.
* **Payload Structure**:
```json
{
  "schema_version": "v1",
  "event_id": "evt_01j0rollbackok",
  "event_type": "watcher.rollback_succeeded",
  "occurred_at": "2026-06-17T02:21:52Z",
  "watcher": {
    "id": 12,
    "name": "api-prod"
  },
  "attempt": {
    "id": 303,
    "kind": "rollback",
    "reason": "auto_after_failed_deploy",
    "status": "succeeded",
    "triggered_by": "agent",
    "target_version": "v1.4.2",
    "from_version": "v1.4.3",
    "failed_target_version": "v1.4.3",
    "failure_phase": "",
    "error": "",
    "parent_attempt_id": 302,
    "root_attempt_id": 302
  },
  "summary": "Rollback of api-prod restored v1.4.2 after v1.4.3 failed"
}
```

---

### `watcher.rollback_failed`

* **Trigger**: Fired when a rollback attempt itself fails (e.g. could not restore the previous junction link or start backup services).
* **Behavior**:
  - **CRITICAL ALERT**: This represents a major system emergency where both the original deploy and the fallback recovery mechanism have failed. Operators should be paged immediately.
* **Payload Structure**:
```json
{
  "schema_version": "v1",
  "event_id": "evt_01j0rollbackfail",
  "event_type": "watcher.rollback_failed",
  "occurred_at": "2026-06-17T02:22:35Z",
  "watcher": {
    "id": 12,
    "name": "api-prod"
  },
  "attempt": {
    "id": 304,
    "kind": "rollback",
    "reason": "auto_after_failed_deploy",
    "status": "failed",
    "triggered_by": "agent",
    "target_version": "v1.4.2",
    "from_version": "v1.4.3",
    "failed_target_version": "v1.4.3",
    "failure_phase": "activate_release",
    "error": "failed to swap current junction",
    "parent_attempt_id": 302,
    "root_attempt_id": 302
  },
  "summary": "Rollback of api-prod to v1.4.2 failed"
}
```

---

### `service.health_changed`

* **Trigger**: Fired when the recorded health status of a service switches (e.g. `healthy` to `unhealthy`, or vice versa).
* **Behavior**:
  - Fired only on state changes. Repetitive checks showing the same status (e.g., healthy to healthy) are suppressed.
  - The `health.source` field indicates what check triggered this state transition (currently `manual` via health API).
* **Payload Structure**:
```json
{
  "schema_version": "v1",
  "event_id": "evt_01j0healthchanged",
  "event_type": "service.health_changed",
  "occurred_at": "2026-06-17T03:02:11Z",
  "watcher": {
    "id": 12,
    "name": "api-prod"
  },
  "service": {
    "id": 87,
    "name": "api-prod-web",
    "service_type": "nssm",
    "health_check_url": "https://api.example.com/health"
  },
  "health": {
    "previous_status": "healthy",
    "current_status": "unhealthy",
    "http_status": 503,
    "error": "unexpected status code 503",
    "checked_at": "2026-06-17T03:02:11Z",
    "source": "manual"
  },
  "summary": "Service api-prod-web health changed from healthy to unhealthy"
}
```

---

### `watcher.webhook_test`

* **Trigger**: Fired when an operator clicks the "Send Test Webhook" button in the Watcher UI dashboard.
* **Behavior**:
  - Always fires and bypasses the business-event subscription checkboxes (is not gated).
  - Follows the identical outbox delivery, retrying, and failure logs pipeline as normal webhooks. Use this to verify network connection and header values.
* **Payload Structure**:
```json
{
  "schema_version": "v1",
  "event_id": "evt_01j0testhook",
  "event_type": "watcher.webhook_test",
  "occurred_at": "2026-06-17T03:30:00Z",
  "watcher": {
    "id": 12,
    "name": "api-prod"
  },
  "triggered_by": "manual",
  "summary": "Webhook test for watcher api-prod"
}
```

---

### `webhook.delivery_exhausted`

* **Trigger**: Fired when an outbound webhook event completely exhausts all its retry attempts and still fails to deliver.
* **Behavior**:
  - Emitted specifically for failed deliveries, not for suppressed events while paused.
  - Non-recursive: if this exhaustion event fails to deliver, Watcher logs the failure locally but does not emit another notification to prevent a cascading loops.
* **Payload Structure**:
```json
{
  "schema_version": "v1",
  "event_id": "evt_01j0exhausted",
  "event_type": "webhook.delivery_exhausted",
  "occurred_at": "2026-06-17T03:35:27Z",
  "watcher": {
    "id": 12,
    "name": "api-prod"
  },
  "failed_delivery": {
    "event_id": "evt_01j0deployfail",
    "event_type": "watcher.deployment_failed",
    "delivery_id": "dlv_01j0deployfail_4",
    "attempt_number": 4,
    "response_status_code": 503,
    "error": "server returned 503",
    "summary": "Deployment of api-prod to v1.4.3 failed during health_check"
  },
  "summary": "Webhook delivery exhausted for watcher.deployment_failed"
}
```

---

## 6. Event Status vs. Delivery Status Reference

To query webhooks via the API, it is helpful to distinguish between the **Event Outbox Status** and the **Delivery Attempt Status**:

### Event Outbox Status (`webhook_events` Table)

- `pending`: The event is queued and waiting for dispatch or currently being tried.
- `delivered`: The event was successfully received by the target URL (returned a `2xx` response).
- `suppressed`: Outbound delivery was paused when this event occurred. The event is stored and won't be sent until the webhook is resumed and the operator selects to replay suppressed events.
- `exhausted`: The event reached the maximum number of retries or received a final non-retryable status (such as `400` or `403`), and has failed permanently.

### Delivery Attempt Status (`webhook_deliveries` Table)

- `pending`: The network dispatch routine has been spawned but the request is not completed yet.
- `retry_wait`: The dispatch failed with a retryable error. The delivery is waiting in the retry queue according to the retry schedule.
- `succeeded`: The request succeeded with a `2xx` response code.
- `failed`: The attempt failed due to network error, timeout, or a non-2xx status code.

---

## 7. Webhook API Reference

Operators can query webhook states and logs using the following endpoints:

* **List Webhook Events for Watcher**
  - Endpoint: `GET /api/watchers/:id/webhook-events`
  - Description: Returns a list of all outbox events queued or delivered for a watcher.

* **List Delivery Attempts for Watcher**
  - Endpoint: `GET /api/watchers/:id/webhook-deliveries?page=1&pageSize=20`
  - Description: Returns a paginated list of all HTTP request attempts.

* **Retrieve Delivery Attempt Details**
  - Endpoint: `GET /api/watchers/:id/webhook-deliveries/:deliveryId`
  - Description: Returns detailed log entries, response status, headers, and duration for a single delivery attempt.

* **Trigger Test Webhook**
  - Endpoint: `POST /api/watchers/:id/webhook/test`
  - Description: Immediately queues a `watcher.webhook_test` event.

* **Resume Webhook Delivery**
  - Endpoint: `POST /api/watchers/:id/webhook/resume`
  - Input: `{"replay_suppressed": true}`
  - Description: Resumes a paused outbox and optionally triggers sequential replay of all `suppressed` events.
