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
