import type { WebhookEventDoc } from './types';
export const event5: WebhookEventDoc = {
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
		{
			field: 'watcher.id',
			type: 'integer',
			meaning: 'Internal ID of the watcher that owns the service.'
		},
		{ field: 'watcher.name', type: 'string', meaning: 'Human-facing watcher name.' },
		{
			field: 'service.id',
			type: 'integer',
			meaning: 'Internal ID of the service whose health changed.'
		},
		{ field: 'service.name', type: 'string', meaning: 'Human-facing service name.' },
		{
			field: 'service.service_type',
			type: 'string',
			meaning: 'Service mode such as nssm or static.'
		},
		{
			field: 'service.health_check_url',
			type: 'string',
			meaning: 'Effective health-check URL used for this service.'
		},
		{
			field: 'health.previous_status',
			type: 'string',
			meaning: 'Previously stored health status before this transition.'
		},
		{
			field: 'health.current_status',
			type: 'string',
			meaning: 'Newly stored health status after this transition.'
		},
		{
			field: 'health.http_status',
			type: 'integer',
			meaning: 'HTTP status returned by the health-check endpoint when available.'
		},
		{
			field: 'health.error',
			type: 'string',
			meaning:
				'Operator-facing health-check error detail. Usually empty when the service is healthy.'
		},
		{
			field: 'health.checked_at',
			type: 'RFC3339 date-time string',
			meaning: 'When the health check result was recorded.'
		},
		{
			field: 'health.source',
			type: 'string',
			meaning:
				'What flow produced the health change, such as manual today and potentially deploy or monitor in the future.'
		},
		{
			field: 'summary',
			type: 'string',
			meaning: 'Human-readable summary for logs and UI. Do not automate from this text.'
		}
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
};
