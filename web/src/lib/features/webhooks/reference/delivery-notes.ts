export const webhookDeliveryNotes = [
	'Webhook delivery is at-least-once. Receivers should dedupe on event_id.',
	'Each HTTP attempt gets its own delivery_id. Replays keep the original event_id and occurred_at, but create a new delivery_id.',
	'Any 2xx response counts as success. Network errors, 429, and 5xx retry. Other 4xx responses are recorded as final failures.',
	'Watcher preserves FIFO order per watcher. Later events wait behind the oldest pending or retry-waiting event for that watcher.',
	'If webhook delivery is paused, new events are stored as suppressed rather than sent. Resume can optionally replay suppressed events in original order.'
] as const;
