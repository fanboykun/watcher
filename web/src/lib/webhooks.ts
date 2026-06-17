export type WebhookSelectionKey =
	| 'notify_version_found'
	| 'notify_deployment_succeeded'
	| 'notify_deployment_failed'
	| 'notify_rollback_succeeded'
	| 'notify_rollback_failed'
	| 'notify_service_health_changed';

export type WebhookSelectionState = Record<WebhookSelectionKey, boolean>;

export type WebhookEventDoc = {
	key: WebhookSelectionKey | null;
	name: string;
	eventType: string;
	anchor: string;
	schemaName: string;
	when: string;
	behavior: string[];
	payload: string[];
	examplePayload: string;
};

export const webhookDocsHref = 'https://github.com/fanboykun/watcher/blob/main/docs/webhooks.md';
export const webhookOpenAPISpecHref = '/webhooks.openapi.yaml';

export function webhookEventDocHref(anchor: string) {
	return `${webhookDocsHref}#${anchor}`;
}

export const webhookEventDocs: WebhookEventDoc[] = [
	{
		key: 'notify_version_found',
		name: 'Version Found',
		eventType: 'watcher.version_found',
		anchor: 'watcher-version-found',
		schemaName: 'WatcherVersionFoundEvent',
		when: 'Emitted when polling discovers a newer remote version than the watcher current version.',
		behavior: [
			'Sent once per watcher per discovered version. Repeated polls for the same version are deduped.',
			'Can fire even when deployment will not proceed. Payload includes will_deploy and block_reason.',
			'This is a discovery event only. It is not linked to a deploy attempt.'
		],
		payload: [
			'watcher.id, watcher.name',
			'version.discovered_version',
			'version.current_version',
			'version.will_deploy',
			'version.block_reason',
			'summary'
		],
		examplePayload: `{
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
}`
	},
	{
		key: 'notify_deployment_succeeded',
		name: 'Deployment Succeeded',
		eventType: 'watcher.deployment_succeeded',
		anchor: 'watcher-deployment-succeeded',
		schemaName: 'WatcherDeploymentSucceededEvent',
		when: 'Emitted when a deploy attempt reaches terminal success.',
		behavior: [
			'Success is emitted only after the deployment flow completes successfully.',
			'Payload uses the deploy attempt record as the source of truth.',
			'Includes who triggered it and the attempt reason so manual redeploys are distinguishable from agent-driven rollouts.'
		],
		payload: [
			'watcher.id, watcher.name',
			'attempt.id, attempt.kind=deploy',
			'attempt.reason, attempt.triggered_by, attempt.status',
			'attempt.target_version, attempt.from_version',
			'summary'
		],
		examplePayload: `{
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
}`
	},
	{
		key: 'notify_deployment_failed',
		name: 'Deployment Failed',
		eventType: 'watcher.deployment_failed',
		anchor: 'watcher-deployment-failed',
		schemaName: 'WatcherDeploymentFailedEvent',
		when: 'Emitted when a deploy attempt reaches terminal failure.',
		behavior: [
			'Failure is emitted as soon as the deploy attempt is marked failed, before any automatic rollback attempt runs.',
			'failure_phase identifies where the deploy failed, such as download, extract, activate_release, start_services, or health_check.',
			'A later rollback event may follow and should be treated as a separate attempt in the same incident chain.'
		],
		payload: [
			'watcher.id, watcher.name',
			'attempt.id, attempt.kind=deploy',
			'attempt.reason, attempt.triggered_by, attempt.status=failed',
			'attempt.target_version, attempt.from_version',
			'attempt.failure_phase, attempt.error',
			'attempt.parent_attempt_id, attempt.root_attempt_id',
			'summary'
		],
		examplePayload: `{
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
}`
	},
	{
		key: 'notify_rollback_succeeded',
		name: 'Rollback Succeeded',
		eventType: 'watcher.rollback_succeeded',
		anchor: 'watcher-rollback-succeeded',
		schemaName: 'WatcherRollbackSucceededEvent',
		when: 'Emitted when a rollback attempt completes successfully.',
		behavior: [
			'Automatic rollback after a failed deploy produces its own rollback attempt and its own webhook event.',
			'The watcher live status returns to healthy even though the attempt history records a rollback.',
			'Includes both the restored version and the failed target version that triggered the rollback.'
		],
		payload: [
			'watcher.id, watcher.name',
			'attempt.id, attempt.kind=rollback',
			'attempt.reason, attempt.triggered_by, attempt.status=succeeded',
			'attempt.target_version',
			'attempt.failed_target_version',
			'attempt.parent_attempt_id, attempt.root_attempt_id',
			'summary'
		],
		examplePayload: `{
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
}`
	},
	{
		key: 'notify_rollback_failed',
		name: 'Rollback Failed',
		eventType: 'watcher.rollback_failed',
		anchor: 'watcher-rollback-failed',
		schemaName: 'WatcherRollbackFailedEvent',
		when: 'Emitted when a rollback attempt itself fails.',
		behavior: [
			'This is distinct from deployment_failed. Receivers should expect deployment_failed first, then rollback_failed if recovery also breaks.',
			'The payload carries rollback attempt identity and the failed target version context.',
			'Use the parent/root attempt linkage to correlate the rollback failure back to the deploy incident.'
		],
		payload: [
			'watcher.id, watcher.name',
			'attempt.id, attempt.kind=rollback',
			'attempt.reason, attempt.triggered_by, attempt.status=failed',
			'attempt.target_version, attempt.failed_target_version',
			'attempt.error, attempt.parent_attempt_id, attempt.root_attempt_id',
			'summary'
		],
		examplePayload: `{
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
}`
	},
	{
		key: 'notify_service_health_changed',
		name: 'Service Health Changed',
		eventType: 'service.health_changed',
		anchor: 'service-health-changed',
		schemaName: 'ServiceHealthChangedEvent',
		when: 'Emitted when the stored health state for a service changes.',
		behavior: [
			'Only state changes emit an event. Repeated healthy -> healthy or unhealthy -> unhealthy checks are suppressed.',
			'Today this is emitted from manual health checks through the service health API. The payload reserves source values for future deploy, rollback, and monitor flows.',
			'This event is service-scoped. watcher context is included, but the primary subject is the service.'
		],
		payload: [
			'watcher.id, watcher.name',
			'service.id, service.name, service.service_type, service.health_check_url',
			'health.previous_status, health.current_status',
			'health.http_status, health.error, health.checked_at, health.source',
			'summary'
		],
		examplePayload: `{
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
}`
	}
];

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
			'Useful for validating URL, bearer token, retry, and delivery history wiring before a real deploy happens.'
		],
		payload: [
			'watcher.id, watcher.name',
			'triggered_by=manual',
			'summary'
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

export const webhookDeliveryNotes = [
	'Webhook delivery is at-least-once. Receivers should dedupe on event_id.',
	'Each HTTP attempt gets its own delivery_id. Replays keep the original event_id and occurred_at, but create a new delivery_id.',
	'Any 2xx response counts as success. Network errors, 429, and 5xx retry. Other 4xx responses are recorded as final failures.',
	'Watcher preserves FIFO order per watcher. Later events wait behind the oldest pending or retry-waiting event for that watcher.',
	'If webhook delivery is paused, new events are stored as suppressed rather than sent. Resume can optionally replay suppressed events in original order.'
] as const;
