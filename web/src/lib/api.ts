// API client for the watcher agent backend
const API_BASE = '/api';

async function request<T>(path: string, options?: RequestInit): Promise<T> {
	const res = await fetch(`${API_BASE}${path}`, {
		headers: { 'Content-Type': 'application/json', ...options?.headers },
		...options
	});
	if (!res.ok) {
		const body = await res.json().catch(() => ({ error: res.statusText }));
		throw new Error(body.error || res.statusText);
	}
	return res.json();
}

// ── Types ────────────────────────────────────────────────────

export interface Watcher {
	id: number;
	name: string;
	service_name: string;
	metadata_url: string;
	check_interval_sec: number;
	download_retries: number;
	install_dir: string;
	hc_enabled: boolean;
	hc_url: string;
	hc_retries: number;
	hc_interval_sec: number;
	hc_timeout_sec: number;
	current_version: string;
	status: string;
	last_checked: string | null;
	last_deployed: string | null;
	last_error: string;
	services: Service[];
	created_at: string;
	updated_at: string;
}

export interface Service {
	id: number;
	watcher_id: number;
	service_type: 'nssm' | 'static';
	windows_service_name: string;
	binary_name: string;
	env_file: string;
	health_check_url: string;
	iis_app_pool: string;
	iis_site_name: string;
	public_url: string;
	created_at: string;
	updated_at: string;
}

export interface ServiceWithWatcher extends Service {
	watcher_name: string;
	install_dir: string;
}

export interface DeployLog {
	id: number;
	watcher_id: number;
	version: string;
	from_version: string;
	status: string;
	error: string;
	duration_ms: number;
	started_at: string | null;
	completed_at: string | null;
}

export interface HealthEvent {
	id: number;
	service_id: number;
	status: string;
	http_status: number;
	error: string;
	checked_at: string | null;
}

export interface SystemStatus {
	status: string;
	version: string;
	uptime_seconds: number;
	uptime_human: string;
	watcher_count: number;
	service_count: number;
	deploys_24h: number;
}

// ── API methods ──────────────────────────────────────────────

export const api = {
	// System
	status: () => request<SystemStatus>('/status'),
	agentLogs: (lines = 100) => request<{ lines: string[] }>(`/logs?lines=${lines}`),

	// Watchers
	listWatchers: () => request<Watcher[]>('/watchers'),
	getWatcher: (id: number) => request<Watcher>(`/watchers/${id}`),
	createWatcher: (data: Partial<Watcher>) => request<Watcher>('/watchers', { method: 'POST', body: JSON.stringify(data) }),
	updateWatcher: (id: number, data: Partial<Watcher>) => request<Watcher>(`/watchers/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
	deleteWatcher: (id: number) => request<{ message: string }>(`/watchers/${id}`, { method: 'DELETE' }),
	triggerCheck: (id: number) => request<{ message: string }>(`/watchers/${id}/check`, { method: 'POST' }),
	redeployWatcher: (id: number) => request<{ message: string }>(`/watchers/${id}/redeploy`, { method: 'POST' }),
	watcherDeploys: (id: number) => request<DeployLog[]>(`/watchers/${id}/deploys`),

	// Services (flat)
	listServices: () => request<ServiceWithWatcher[]>('/services'),
	getService: (id: number) => request<{ service: Service; watcher: Watcher }>(`/services/${id}`),
	startService: (id: number) => request<{ message: string }>(`/services/${id}/start`, { method: 'POST' }),
	stopService: (id: number) => request<{ message: string }>(`/services/${id}/stop`, { method: 'POST' }),
	restartService: (id: number) => request<{ message: string }>(`/services/${id}/restart`, { method: 'POST' }),
	serviceHealth: (id: number) => request<{ status: string; http_status: number; error: string }>(`/services/${id}/health`),
	healthHistory: (id: number, limit = 50) => request<HealthEvent[]>(`/services/${id}/health/history?limit=${limit}`),
	serviceLogs: (id: number, lines = 100, type = 'out') => request<{ lines: string[] }>(`/services/${id}/logs?lines=${lines}&type=${type}`),
	serviceDeploys: (id: number) => request<DeployLog[]>(`/services/${id}/deploys`),

	// Services (nested under watcher)
	createService: (watcherId: number, data: Partial<Service>) => request<Service>(`/watchers/${watcherId}/services`, { method: 'POST', body: JSON.stringify(data) }),
	deleteService: (watcherId: number, serviceId: number) => request<{ message: string }>(`/watchers/${watcherId}/services/${serviceId}`, { method: 'DELETE' })
};
