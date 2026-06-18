export type WebhookSelectionKey =
	| 'notify_version_found'
	| 'notify_deployment_succeeded'
	| 'notify_deployment_failed'
	| 'notify_rollback_succeeded'
	| 'notify_rollback_failed'
	| 'notify_service_health_changed';

export type WebhookSelectionState = Record<WebhookSelectionKey, boolean>;

export type WebhookFieldContract = {
	field: string;
	type: string;
	meaning: string;
};

export type WebhookEventDoc = {
	key: WebhookSelectionKey | null;
	name: string;
	eventType: string;
	anchor: string;
	schemaName: string;
	when: string;
	behavior: string[];
	payload: string[];
	fields: WebhookFieldContract[];
	interpretation?: string[];
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
		fields: [
			{ field: 'watcher.id', type: 'integer', meaning: 'Internal ID of the watcher that emitted the event.' },
			{ field: 'watcher.name', type: 'string', meaning: 'Human-facing watcher name.' },
			{ field: 'version.discovered_version', type: 'string', meaning: 'The newer remote version string found during polling.' },
			{ field: 'version.current_version', type: 'string', meaning: 'The watcher current active version when discovery happened.' },
			{ field: 'version.will_deploy', type: 'boolean', meaning: 'Whether Watcher can continue toward deployment after discovery.' },
			{ field: 'version.block_reason', type: 'string', meaning: 'Operator-facing reason deployment will not proceed when will_deploy is false. Usually empty when there is no block.' },
			{ field: 'summary', type: 'string', meaning: 'Human-readable summary for logs and UI. Do not automate from this text.' }
		],
		interpretation: [
			'This is a discovery event, not a deploy-attempt event.',
			'will_deploy=true means rollout may proceed after discovery.',
			'will_deploy=false means the discovery is real, but rollout is blocked and block_reason explains why.'
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
		fields: [
			{ field: 'watcher.id', type: 'integer', meaning: 'Internal ID of the watcher that emitted the event.' },
			{ field: 'watcher.name', type: 'string', meaning: 'Human-facing watcher name.' },
			{ field: 'attempt.id', type: 'integer', meaning: 'Unique ID of this deploy attempt record.' },
			{ field: 'attempt.kind', type: 'string', meaning: 'Always deploy for this event.' },
			{ field: 'attempt.reason', type: 'string', meaning: 'Why the deploy was started, such as new_version_found or manual_redeploy.' },
			{ field: 'attempt.triggered_by', type: 'string', meaning: 'Who initiated the deploy flow, such as agent or manual.' },
			{ field: 'attempt.status', type: 'string', meaning: 'Always succeeded for this event.' },
			{ field: 'attempt.target_version', type: 'string', meaning: 'Version that was successfully deployed.' },
			{ field: 'attempt.from_version', type: 'string', meaning: 'Version that was active before the deploy started.' },
			{ field: 'summary', type: 'string', meaning: 'Human-readable summary for logs and UI. Do not automate from this text.' }
		],
		interpretation: [
			'This means the deploy reached terminal success.',
			'If health checks are part of the deploy flow, success is emitted only after they pass.',
			'Use attempt.id and root_attempt_id from the payload example if you need to correlate later rollback activity.'
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
		fields: [
			{ field: 'watcher.id', type: 'integer', meaning: 'Internal ID of the watcher that emitted the event.' },
			{ field: 'watcher.name', type: 'string', meaning: 'Human-facing watcher name.' },
			{ field: 'attempt.id', type: 'integer', meaning: 'Unique ID of this failed deploy attempt record.' },
			{ field: 'attempt.kind', type: 'string', meaning: 'Always deploy for this event.' },
			{ field: 'attempt.reason', type: 'string', meaning: 'Why the deploy was started, such as new_version_found or manual_redeploy.' },
			{ field: 'attempt.triggered_by', type: 'string', meaning: 'Who initiated the deploy flow, such as agent or manual.' },
			{ field: 'attempt.status', type: 'string', meaning: 'Always failed for this event.' },
			{ field: 'attempt.target_version', type: 'string', meaning: 'Version Watcher tried to deploy.' },
			{ field: 'attempt.from_version', type: 'string', meaning: 'Version that was active before the failed deploy started.' },
			{ field: 'attempt.failure_phase', type: 'string', meaning: 'Step where the deploy failed, such as download, extract, activate_release, start_services, or health_check.' },
			{ field: 'attempt.error', type: 'string', meaning: 'Operator-facing failure detail. Do not automate from this text.' },
			{ field: 'attempt.parent_attempt_id', type: 'integer | null', meaning: 'Immediate parent attempt if this deploy belongs to another attempt chain. Usually null for the initial failed deploy.' },
			{ field: 'attempt.root_attempt_id', type: 'integer', meaning: 'Root incident chain ID used to correlate related attempts.' },
			{ field: 'summary', type: 'string', meaning: 'Human-readable summary for logs and UI. Do not automate from this text.' }
		],
		interpretation: [
			'This event is emitted before any automatic rollback attempt runs.',
			'failure_phase is the structured field to automate against, not the free-form error text.',
			'A later rollback event may follow as part of the same incident chain.'
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
		fields: [
			{ field: 'watcher.id', type: 'integer', meaning: 'Internal ID of the watcher that emitted the event.' },
			{ field: 'watcher.name', type: 'string', meaning: 'Human-facing watcher name.' },
			{ field: 'attempt.id', type: 'integer', meaning: 'Unique ID of this rollback attempt record.' },
			{ field: 'attempt.kind', type: 'string', meaning: 'Always rollback for this event.' },
			{ field: 'attempt.reason', type: 'string', meaning: 'Why the rollback was started, such as auto_after_failed_deploy or manual_rollback.' },
			{ field: 'attempt.triggered_by', type: 'string', meaning: 'Who initiated the rollback flow, such as agent or manual.' },
			{ field: 'attempt.status', type: 'string', meaning: 'Always succeeded for this event.' },
			{ field: 'attempt.target_version', type: 'string', meaning: 'Version that Watcher restored successfully. This is the rollback destination.' },
			{ field: 'attempt.failed_target_version', type: 'string', meaning: 'Version whose failure triggered the rollback attempt.' },
			{ field: 'attempt.parent_attempt_id', type: 'integer | null', meaning: 'Immediate parent attempt that caused this rollback. Usually the failed deploy attempt for automatic rollback.' },
			{ field: 'attempt.root_attempt_id', type: 'integer', meaning: 'Root incident chain ID used to correlate deploy failure and rollback success together.' },
			{ field: 'summary', type: 'string', meaning: 'Human-readable summary for logs and UI. Do not automate from this text.' }
		],
		interpretation: [
			'target_version is the version that was restored.',
			'failed_target_version is the bad version that triggered recovery.',
			'Use parent_attempt_id or root_attempt_id to correlate this rollback with the original failed deploy.'
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
		fields: [
			{ field: 'watcher.id', type: 'integer', meaning: 'Internal ID of the watcher that emitted the event.' },
			{ field: 'watcher.name', type: 'string', meaning: 'Human-facing watcher name.' },
			{ field: 'attempt.id', type: 'integer', meaning: 'Unique ID of this rollback attempt record.' },
			{ field: 'attempt.kind', type: 'string', meaning: 'Always rollback for this event.' },
			{ field: 'attempt.reason', type: 'string', meaning: 'Why the rollback was started, such as auto_after_failed_deploy or manual_rollback.' },
			{ field: 'attempt.triggered_by', type: 'string', meaning: 'Who initiated the rollback flow, such as agent or manual.' },
			{ field: 'attempt.status', type: 'string', meaning: 'Always failed for this event.' },
			{ field: 'attempt.target_version', type: 'string', meaning: 'Version Watcher attempted to restore. This is the intended rollback destination.' },
			{ field: 'attempt.failed_target_version', type: 'string', meaning: 'Version whose failure led to this rollback attempt. In automatic recovery this is usually the bad deployment version.' },
			{ field: 'attempt.error', type: 'string', meaning: 'Operator-facing rollback failure detail. Do not automate from this text.' },
			{ field: 'attempt.parent_attempt_id', type: 'integer | null', meaning: 'Immediate parent attempt that caused this rollback. Usually the failed deploy attempt for automatic rollback.' },
			{ field: 'attempt.root_attempt_id', type: 'integer', meaning: 'Root incident chain ID used to correlate deploy failure and rollback failure together.' },
			{ field: 'summary', type: 'string', meaning: 'Human-readable summary for logs and UI. Do not automate from this text.' }
		],
		interpretation: [
			'This is separate from watcher.deployment_failed; expect the deploy failure event first, then this if recovery also breaks.',
			'target_version is what Watcher tried to restore.',
			'failed_target_version is the bad version that triggered recovery.'
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
		fields: [
			{ field: 'watcher.id', type: 'integer', meaning: 'Internal ID of the watcher that owns the service.' },
			{ field: 'watcher.name', type: 'string', meaning: 'Human-facing watcher name.' },
			{ field: 'service.id', type: 'integer', meaning: 'Internal ID of the service whose health changed.' },
			{ field: 'service.name', type: 'string', meaning: 'Human-facing service name.' },
			{ field: 'service.service_type', type: 'string', meaning: 'Service mode such as nssm or static.' },
			{ field: 'service.health_check_url', type: 'string', meaning: 'Effective health-check URL used for this service.' },
			{ field: 'health.previous_status', type: 'string', meaning: 'Previously stored health status before this transition.' },
			{ field: 'health.current_status', type: 'string', meaning: 'Newly stored health status after this transition.' },
			{ field: 'health.http_status', type: 'integer', meaning: 'HTTP status returned by the health-check endpoint when available.' },
			{ field: 'health.error', type: 'string', meaning: 'Operator-facing health-check error detail. Usually empty when the service is healthy.' },
			{ field: 'health.checked_at', type: 'RFC3339 date-time string', meaning: 'When the health check result was recorded.' },
			{ field: 'health.source', type: 'string', meaning: 'What flow produced the health change, such as manual today and potentially deploy or monitor in the future.' },
			{ field: 'summary', type: 'string', meaning: 'Human-readable summary for logs and UI. Do not automate from this text.' }
		],
		interpretation: [
			'This event is emitted only when the stored health state changes.',
			'The primary subject is the service; watcher context is included for correlation.',
			'Automate from current_status and previous_status, not from summary or error text.'
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
		fields: [
			{ field: 'watcher.id', type: 'integer', meaning: 'Internal ID of the watcher that emitted the test event.' },
			{ field: 'watcher.name', type: 'string', meaning: 'Human-facing watcher name.' },
			{ field: 'triggered_by', type: 'string', meaning: 'Who triggered the test event. Currently manual.' },
			{ field: 'summary', type: 'string', meaning: 'Human-readable summary for logs and UI. Do not automate from this text.' }
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
			{ field: 'watcher.id', type: 'integer', meaning: 'Internal ID of the watcher whose webhook delivery exhausted retries.' },
			{ field: 'watcher.name', type: 'string', meaning: 'Human-facing watcher name.' },
			{ field: 'failed_delivery.event_id', type: 'string', meaning: 'Business-event ID of the original event that could not be delivered.' },
			{ field: 'failed_delivery.event_type', type: 'string', meaning: 'Event type of the original undelivered event, such as watcher.deployment_failed.' },
			{ field: 'failed_delivery.delivery_id', type: 'string', meaning: 'Delivery-attempt ID of the final failed HTTP attempt.' },
			{ field: 'failed_delivery.attempt_number', type: 'integer', meaning: 'Ordinal number of the final failed delivery attempt.' },
			{ field: 'failed_delivery.response_status_code', type: 'integer', meaning: 'HTTP status code from the final failed attempt when one was received.' },
			{ field: 'failed_delivery.error', type: 'string', meaning: 'Operator-facing delivery failure detail for the final attempt.' },
			{ field: 'failed_delivery.summary', type: 'string', meaning: 'Human-readable summary of the original event that could not be delivered.' },
			{ field: 'summary', type: 'string', meaning: 'Human-readable summary for logs and UI. Do not automate from this text.' }
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

export const webhookDeliveryNotes = [
	'Webhook delivery is at-least-once. Receivers should dedupe on event_id.',
	'Each HTTP attempt gets its own delivery_id. Replays keep the original event_id and occurred_at, but create a new delivery_id.',
	'Any 2xx response counts as success. Network errors, 429, and 5xx retry. Other 4xx responses are recorded as final failures.',
	'Watcher preserves FIFO order per watcher. Later events wait behind the oldest pending or retry-waiting event for that watcher.',
	'If webhook delivery is paused, new events are stored as suppressed rather than sent. Resume can optionally replay suppressed events in original order.'
] as const;
