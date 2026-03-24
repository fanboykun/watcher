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

export interface InspectRepoResponse {
	latest_version: string;
	published_at: string;
	assets: string[];
}

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
	paused: boolean;
	max_kept_versions: number;
	current_version: string;
	max_ignored_version: string;
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
	env_content: string;
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
	github_deployment_id: number;
	logs: string | null;
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

export interface PollEvent {
	id: number;
	watcher_id: number;
	checked_at: string;
	status: string;
	remote_version: string;
	error: string;
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

export interface ReleaseInfo {
	version: string;
	mod_time: string;
	size_bytes: number;
	size_human: string;
	is_current: boolean;
}

export interface SelfVersionResponse {
	version: string;
	go_version: string;
	os: string;
	arch: string;
	executable: string;
}

export interface SelfUpdateCheckResponse {
	update_available: boolean;
	current_version: string;
	latest_version: string;
	release_notes: string;
	download_url: string;
	published_at: string;
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
	watcherDeployLog: (id: number, logId: number) => request<DeployLog>(`/watchers/${id}/deploys/${logId}`),
	watcherVersions: async (id: number) => {
		const res = await request<{ versions: ReleaseInfo[] }>(`/watchers/${id}/versions`);
		return res.versions;
	},
	rollbackWatcher: (id: number, version: string) => request<{ message: string }>(`/watchers/${id}/rollback`, { method: 'POST', body: JSON.stringify({ version }) }),
	resumeWatcherUpdates: (id: number) => request<{ message: string }>(`/watchers/${id}/resume`, { method: 'POST' }),
	deleteWatcherVersion: (id: number, version: string) => request<{ message: string }>(`/watchers/${id}/versions/${version}`, { method: 'DELETE' }),
	watcherPolls: (id: number, page = 1, pageSize = 10, status = 'all') => request<{ data: PollEvent[], total: number, page: number, pageSize: number }>(`/watchers/${id}/polls?page=${page}&pageSize=${pageSize}&status=${status}`),

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
	syncServiceEnv: (id: number, envContent: string) => request<{ message: string }>(`/services/${id}/env`, { method: 'PUT', body: JSON.stringify({ env_content: envContent }) }),

	// Services (nested under watcher)
	createService: (watcherId: number, data: Partial<Service>) => request<Service>(`/watchers/${watcherId}/services`, { method: 'POST', body: JSON.stringify(data) }),
	deleteService: (watcherId: number, serviceId: number) => request<{ message: string }>(`/watchers/${watcherId}/services/${serviceId}`, { method: 'DELETE' }),

	// GitHub Integration
	inspectRepo: (repoUrl: string) => request<InspectRepoResponse>('/github/inspect', { method: 'POST', body: JSON.stringify({ repo_url: repoUrl }) }),

	// Agent Self-Management
	selfVersion: () => request<SelfVersionResponse>('/self/version'),
	selfUpdateCheck: () => request<SelfUpdateCheckResponse>('/self/update-check'),
	selfUpdate: () => request<{ message: string }>('/self/update', { method: 'POST' }),
	selfUninstall: () => request<{ script: string }>('/self/uninstall', { method: 'POST' })
};
