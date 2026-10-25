import type { WebhookEventDoc } from './types';
export const event0: WebhookEventDoc = {
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
		{
			field: 'watcher.id',
			type: 'integer',
			meaning: 'Internal ID of the watcher that emitted the event.'
		},
		{ field: 'watcher.name', type: 'string', meaning: 'Human-facing watcher name.' },
		{
			field: 'version.discovered_version',
			type: 'string',
			meaning: 'The newer remote version string found during polling.'
		},
		{
			field: 'version.current_version',
			type: 'string',
			meaning: 'The watcher current active version when discovery happened.'
		},
		{
			field: 'version.will_deploy',
			type: 'boolean',
			meaning: 'Whether Watcher can continue toward deployment after discovery.'
		},
		{
			field: 'version.block_reason',
			type: 'string',
			meaning:
				'Operator-facing reason deployment will not proceed when will_deploy is false. Usually empty when there is no block.'
		},
		{
			field: 'summary',
			type: 'string',
			meaning: 'Human-readable summary for logs and UI. Do not automate from this text.'
		}
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
};
