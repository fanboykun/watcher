import type { WebhookEventDoc } from './types';
export const event3: WebhookEventDoc = {
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
		{
			field: 'watcher.id',
			type: 'integer',
			meaning: 'Internal ID of the watcher that emitted the event.'
		},
		{ field: 'watcher.name', type: 'string', meaning: 'Human-facing watcher name.' },
		{
			field: 'attempt.id',
			type: 'integer',
			meaning: 'Unique ID of this rollback attempt record.'
		},
		{ field: 'attempt.kind', type: 'string', meaning: 'Always rollback for this event.' },
		{
			field: 'attempt.reason',
			type: 'string',
			meaning: 'Why the rollback was started, such as auto_after_failed_deploy or manual_rollback.'
		},
		{
			field: 'attempt.triggered_by',
			type: 'string',
			meaning: 'Who initiated the rollback flow, such as agent or manual.'
		},
		{ field: 'attempt.status', type: 'string', meaning: 'Always succeeded for this event.' },
		{
			field: 'attempt.target_version',
			type: 'string',
			meaning: 'Version that Watcher restored successfully. This is the rollback destination.'
		},
		{
			field: 'attempt.failed_target_version',
			type: 'string',
			meaning: 'Version whose failure triggered the rollback attempt.'
		},
		{
			field: 'attempt.parent_attempt_id',
			type: 'integer | null',
			meaning:
				'Immediate parent attempt that caused this rollback. Usually the failed deploy attempt for automatic rollback.'
		},
		{
			field: 'attempt.root_attempt_id',
			type: 'integer',
			meaning:
				'Root incident chain ID used to correlate deploy failure and rollback success together.'
		},
		{
			field: 'summary',
			type: 'string',
			meaning: 'Human-readable summary for logs and UI. Do not automate from this text.'
		}
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
};
