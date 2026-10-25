import type { WebhookEventDoc } from './types';
export const event1: WebhookEventDoc = {
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
		{
			field: 'watcher.id',
			type: 'integer',
			meaning: 'Internal ID of the watcher that emitted the event.'
		},
		{ field: 'watcher.name', type: 'string', meaning: 'Human-facing watcher name.' },
		{ field: 'attempt.id', type: 'integer', meaning: 'Unique ID of this deploy attempt record.' },
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
		{ field: 'attempt.status', type: 'string', meaning: 'Always succeeded for this event.' },
		{
			field: 'attempt.target_version',
			type: 'string',
			meaning: 'Version that was successfully deployed.'
		},
		{
			field: 'attempt.from_version',
			type: 'string',
			meaning: 'Version that was active before the deploy started.'
		},
		{
			field: 'summary',
			type: 'string',
			meaning: 'Human-readable summary for logs and UI. Do not automate from this text.'
		}
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
};
