export const webhookSystemEventDocs = [
	{
		name: 'Webhook Test',
		eventType: 'watcher.webhook_test',
		anchor: 'watcher-webhook-test',
		schemaName: 'WatcherWebhookTestEvent',
		when: 'Queued when an operator clicks Send Test Webhook.',
		behavior: [
			'Uses the exact same outbox, retry, pause, and delivery history pipeline as real webhook events.',
			'Not controlled by the business-event subscription checkboxes.',
			'Useful for validating URL, signing secret, retry, and delivery history wiring before a real deploy happens.'
		],
		payload: ['watcher.id, watcher.name', 'triggered_by=manual', 'summary'],
		fields: [
			{
				field: 'watcher.id',
				type: 'integer',
				meaning: 'Internal ID of the watcher that emitted the test event.'
			},
			{ field: 'watcher.name', type: 'string', meaning: 'Human-facing watcher name.' },
			{
				field: 'triggered_by',
				type: 'string',
				meaning: 'Who triggered the test event. Currently manual.'
			},
			{
				field: 'summary',
				type: 'string',
				meaning: 'Human-readable summary for logs and UI. Do not automate from this text.'
			}
		],
		interpretation: [
			'This is a synthetic validation event, not a business lifecycle event.',
			'It uses the same outbox and delivery pipeline as real events.'
		],
		examplePayload: `{
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
}`
	},
	{
		name: 'Delivery Exhausted',
		eventType: 'webhook.delivery_exhausted',
		anchor: 'webhook-delivery-exhausted',
		schemaName: 'WebhookDeliveryExhaustedEvent',
		when: 'Queued after a previously emitted webhook event finishes its final retry and still fails.',
		behavior: [
			'This is emitted for exhausted attempted deliveries only, not for events that were merely suppressed while webhook delivery was paused.',
			'The exhausted event includes a compact summary of the original failed event and delivery attempt.',
			'It is non-recursive: if webhook.delivery_exhausted itself cannot be delivered, Watcher records that internally and does not emit another exhaustion event.'
		],
		payload: [
			'watcher.id, watcher.name',
			'failed_delivery.event_id, failed_delivery.event_type',
			'failed_delivery.delivery_id, failed_delivery.attempt_number',
			'failed_delivery.response_status_code, failed_delivery.error',
			'failed_delivery.summary',
			'summary'
		],
		fields: [
			{
				field: 'watcher.id',
				type: 'integer',
				meaning: 'Internal ID of the watcher whose webhook delivery exhausted retries.'
			},
			{ field: 'watcher.name', type: 'string', meaning: 'Human-facing watcher name.' },
			{
				field: 'failed_delivery.event_id',
				type: 'string',
				meaning: 'Business-event ID of the original event that could not be delivered.'
			},
			{
				field: 'failed_delivery.event_type',
				type: 'string',
				meaning: 'Event type of the original undelivered event, such as watcher.deployment_failed.'
			},
			{
				field: 'failed_delivery.delivery_id',
				type: 'string',
				meaning: 'Delivery-attempt ID of the final failed HTTP attempt.'
			},
			{
				field: 'failed_delivery.attempt_number',
				type: 'integer',
				meaning: 'Ordinal number of the final failed delivery attempt.'
			},
			{
				field: 'failed_delivery.response_status_code',
				type: 'integer',
				meaning: 'HTTP status code from the final failed attempt when one was received.'
			},
			{
				field: 'failed_delivery.error',
				type: 'string',
				meaning: 'Operator-facing delivery failure detail for the final attempt.'
			},
			{
				field: 'failed_delivery.summary',
				type: 'string',
				meaning: 'Human-readable summary of the original event that could not be delivered.'
			},
			{
				field: 'summary',
				type: 'string',
				meaning: 'Human-readable summary for logs and UI. Do not automate from this text.'
			}
		],
		interpretation: [
			'This event describes a webhook delivery failure, not a service or deployment lifecycle change.',
			'It is non-recursive; Watcher does not emit another exhausted event if this one also fails.',
			'Use failed_delivery.event_id to correlate back to the original business event.'
		],
		examplePayload: `{
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
}`
	}
] as const;
