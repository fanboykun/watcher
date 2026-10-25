export type * from './features/webhooks/reference/types';
import { asset } from '$app/paths';
export const webhookDocsHref = 'https://github.com/fanboykun/watcher/blob/main/docs/webhooks.md';
export const webhookOpenAPISpecHref = asset('/webhooks.openapi.yaml');
export function webhookEventDocHref(anchor: string) {
	return `${webhookDocsHref}#${anchor}`;
}
import { event0 } from './features/webhooks/reference/version-found';
import { event1 } from './features/webhooks/reference/deployment-succeeded';
import { event2 } from './features/webhooks/reference/deployment-failed';
import { event3 } from './features/webhooks/reference/rollback-succeeded';
import { event4 } from './features/webhooks/reference/rollback-failed';
import { event5 } from './features/webhooks/reference/service.health-changed';
export const webhookEventDocs = [event0, event1, event2, event3, event4, event5];
export { webhookSystemEventDocs } from './features/webhooks/reference/system-events';
export { webhookDeliveryNotes } from './features/webhooks/reference/delivery-notes';
