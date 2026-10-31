export interface WebhookEvent {
	id: number;
	watcher_id: number;
	event_id: string;
	schema_version: string;
	event_type: string;
	dedupe_key: string;
	status: string;
	summary: string;
	payload: string;
	occurred_at: string;
	suppressed_at: string | null;
	last_delivery_at: string | null;
	created_at: string;
	updated_at: string;
}

export interface WebhookDelivery {
	id: number;
	watcher_id: number;
	webhook_event_id: number;
	event_id: string;
	event_type: string;
	summary: string;
	delivery_id: string;
	status: string;
	attempt_number: number;
	response_status_code: number;
	response_body: string;
	error: string;
	resolved_url: string;
	auth_type: string;
	secret_source: string;
	webhook_timestamp: number;
	webhook_signature: string;
	next_retry_at: string | null;
	last_attempt_at: string | null;
	completed_at: string | null;
	replayed_at: string | null;
	replayed_by: string;
	created_at: string;
	updated_at: string;
}

export interface WebhookDeliveryDetails {
	delivery: WebhookDelivery;
	event: {
		id: number;
		event_id: string;
		event_type: string;
		schema_version: string;
		status: string;
		summary: string;
		occurred_at: string;
		payload: unknown;
	};
	request: {
		url: string;
		auth_type: string;
		secret_source: string;
		headers: {
			content_type: string;
			webhook_id: string;
			webhook_timestamp: number;
			webhook_signature: string;
			x_watcher_event: string;
			x_watcher_delivery_id: string;
		};
	};
}
