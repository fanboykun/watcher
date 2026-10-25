import type { WebhookEventDoc } from './types';
export const event4: WebhookEventDoc = {
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
		{ field: 'attempt.status', type: 'string', meaning: 'Always failed for this event.' },
		{
			field: 'attempt.target_version',
			type: 'string',
			meaning: 'Version Watcher attempted to restore. This is the intended rollback destination.'
		},
		{
			field: 'attempt.failed_target_version',
			type: 'string',
			meaning:
				'Version whose failure led to this rollback attempt. In automatic recovery this is usually the bad deployment version.'
		},
		{
			field: 'attempt.error',
			type: 'string',
			meaning: 'Operator-facing rollback failure detail. Do not automate from this text.'
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
				'Root incident chain ID used to correlate deploy failure and rollback failure together.'
		},
		{
			field: 'summary',
			type: 'string',
			meaning: 'Human-readable summary for logs and UI. Do not automate from this text.'
		}
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
};
