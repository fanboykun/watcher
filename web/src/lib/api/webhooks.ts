import type {
	WebhookEvent,
	WebhookDelivery,
	WebhookDeliveryDetails,
	PaginatedResponse
} from './types';
import { request } from './transport';
function normalizePaginatedResponse<T>(response: PaginatedResponse<T>): PaginatedResponse<T> {
	return {
		...response,
		data: Array.isArray(response?.data) ? response.data : []
	};
}
export const webhooksApi = {
	watcherWebhookEvents: (id: number) => request<WebhookEvent[]>(`/watchers/${id}/webhook-events`),
	watcherWebhookDeliveries: async (id: number, page = 1, pageSize = 20, signal?: AbortSignal) =>
		normalizePaginatedResponse(
			await request<PaginatedResponse<WebhookDelivery>>(
				`/watchers/${id}/webhook-deliveries?page=${page}&pageSize=${pageSize}`,
				{ signal }
			)
		),
	watcherWebhookDelivery: (id: number, deliveryId: number, signal?: AbortSignal) =>
		request<WebhookDeliveryDetails>(`/watchers/${id}/webhook-deliveries/${deliveryId}`, { signal }),
	sendWatcherWebhookTest: (id: number) =>
		request<{ message: string }>(`/watchers/${id}/webhook/test`, { method: 'POST' }),
	resumeWatcherWebhook: (id: number, replaySuppressed = false) =>
		request<{ message: string; replay_suppressed: boolean }>(`/watchers/${id}/webhook/resume`, {
			method: 'POST',
			body: JSON.stringify({ replay_suppressed: replaySuppressed })
		})
};
