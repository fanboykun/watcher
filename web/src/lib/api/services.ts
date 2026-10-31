import type {
	Watcher,
	Service,
	ServiceWritePayload,
	ServiceWithWatcher,
	DeployLog,
	HealthEvent,
	PaginatedResponse
} from './types';
import { request } from './transport';
export const servicesApi = {
	listServices: (signal?: AbortSignal) => request<ServiceWithWatcher[]>('/services', { signal }),
	getService: (id: number, signal?: AbortSignal) =>
		request<{ service: Service; watcher: Watcher }>(`/services/${id}`, { signal }),
	serviceStatus: (id: number, signal?: AbortSignal) =>
		request<Service>(`/services/${id}/status`, { signal }),
	startService: (id: number) =>
		request<{ message: string }>(`/services/${id}/start`, { method: 'POST' }),
	stopService: (id: number) =>
		request<{ message: string }>(`/services/${id}/stop`, { method: 'POST' }),
	restartService: (id: number) =>
		request<{ message: string }>(`/services/${id}/restart`, { method: 'POST' }),
	serviceHealth: (id: number) =>
		request<{ status: string; http_status: number; error: string }>(`/services/${id}/health`),
	healthHistory: (id: number, limit = 50, signal?: AbortSignal) =>
		request<HealthEvent[]>(`/services/${id}/health/history?limit=${limit}`, { signal }),
	serviceLogs: (id: number, lines = 100, type = 'out', signal?: AbortSignal) =>
		request<{ lines: string[] }>(`/services/${id}/logs?lines=${lines}&type=${type}`, { signal }),
	serviceDeploys: (id: number, page = 1, pageSize = 10, signal?: AbortSignal) =>
		request<PaginatedResponse<DeployLog>>(
			`/services/${id}/deploys?page=${page}&pageSize=${pageSize}`,
			{ signal }
		),
	syncServiceEnv: (id: number, envContent: string) =>
		request<{ message: string }>(`/services/${id}/env`, {
			method: 'PUT',
			body: JSON.stringify({ env_content: envContent })
		}),
	createService: (watcherId: number, data: ServiceWritePayload) =>
		request<Service>(`/watchers/${watcherId}/services`, {
			method: 'POST',
			body: JSON.stringify(data)
		}),
	updateService: (watcherId: number, serviceId: number, data: ServiceWritePayload) =>
		request<Service>(`/watchers/${watcherId}/services/${serviceId}`, {
			method: 'PUT',
			body: JSON.stringify(data)
		}),
	deleteService: (watcherId: number, serviceId: number) =>
		request<{ message: string }>(`/watchers/${watcherId}/services/${serviceId}`, {
			method: 'DELETE'
		})
};
