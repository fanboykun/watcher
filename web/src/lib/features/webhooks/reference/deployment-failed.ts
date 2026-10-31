import type { WebhookEventDoc } from './types';
export const event2: WebhookEventDoc = {
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
		{
			field: 'watcher.id',
			type: 'integer',
			meaning: 'Internal ID of the watcher that emitted the event.'
		},
		{ field: 'watcher.name', type: 'string', meaning: 'Human-facing watcher name.' },
		{
			field: 'attempt.id',
			type: 'integer',
			meaning: 'Unique ID of this failed deploy attempt record.'
		},
		{ field: 'attempt.kind', type: 'string', meaning: 'Always deploy for this event.' },
		{
			field: 'attempt.reason',
			type: 'string',
			meaning: 'Why the deploy was started, such as new_version_found or manual_redeploy.'
		},
		{
			field: 'attempt.triggered_by',
			type: 'string',
			meaning: 'Who initiated the deploy flow, such as agent or manual.'
		},
		{ field: 'attempt.status', type: 'string', meaning: 'Always failed for this event.' },
		{
			field: 'attempt.target_version',
			type: 'string',
			meaning: 'Version Watcher tried to deploy.'
		},
		{
			field: 'attempt.from_version',
			type: 'string',
			meaning: 'Version that was active before the failed deploy started.'
		},
		{
			field: 'attempt.failure_phase',
			type: 'string',
			meaning:
				'Step where the deploy failed, such as download, extract, activate_release, start_services, or health_check.'
		},
		{
			field: 'attempt.error',
			type: 'string',
			meaning: 'Operator-facing failure detail. Do not automate from this text.'
		},
		{
			field: 'attempt.parent_attempt_id',
			type: 'integer | null',
			meaning:
				'Immediate parent attempt if this deploy belongs to another attempt chain. Usually null for the initial failed deploy.'
		},
		{
			field: 'attempt.root_attempt_id',
			type: 'integer',
			meaning: 'Root incident chain ID used to correlate related attempts.'
		},
		{
			field: 'summary',
			type: 'string',
			meaning: 'Human-readable summary for logs and UI. Do not automate from this text.'
		}
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
};
