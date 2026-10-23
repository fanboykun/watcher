import type {
	AgentLogSource,
	LogFileResponse,
	AuthStatusResponse,
	AuthBootstrapResponse,
	InspectRepoResponse,
	Watcher,
	WatcherWritePayload,
	ServiceType,
	Service,
	ServiceWritePayload,
	ServiceConfigFile,
	ServiceWithWatcher,
	ServiceConfigRevision,
	ConfigSnapshot,
	WatcherCandidateResponse,
	DeployLog,
	RollbackResponse,
	HealthEvent,
	WebhookEvent,
	WebhookDelivery,
	WebhookDeliveryDetails,
	PollEvent,
	PaginatedResponse,
	SystemStatus,
	ReleaseInfo,
	SelfVersionResponse,
	SelfUpdateCheckResponse,
	SelfConfigResponse,
	UpdateSelfConfigRequest,
	CatalogDownload,
	ReleaseCatalogResponse
} from './api/types';
import { request, getAuthPassword, setAuthPassword, clearAuthPassword } from './api/transport';
import { openAuthenticatedEventStream } from './api/streams';

export * from './api/types';
export {
	APIRequestError,
	errorMessageFromResponse,
	getAuthPassword,
	setAuthPassword,
	clearAuthPassword
} from './api/transport';
export { openAuthenticatedEventStream, type AuthenticatedEventStream } from './api/streams';

function normalizePaginatedResponse<T>(response: PaginatedResponse<T>): PaginatedResponse<T> {
	return {
		...response,
		data: Array.isArray(response?.data) ? response.data : []
	};
}

export function isIISService(serviceType: Service['service_type'] | ServiceType): boolean {
	return serviceType === 'iis' || serviceType === 'static';
}

export function serviceTypeLabel(serviceType: Service['service_type'] | ServiceType): string {
	return isIISService(serviceType) ? 'IIS Site' : 'Binary (NSSM)';
}

export function iisAppKindLabel(kind: string): string {
	switch (kind) {
		case 'php':
			return 'PHP';
		case 'aspnet_classic':
			return 'ASP.NET Classic';
		default:
			return 'Static Site';
	}
}

// ── API methods ──────────────────────────────────────────────

export const auth = {
	getPassword: getAuthPassword,
	setPassword: setAuthPassword,
	clearPassword: clearAuthPassword,
	hasPassword: () => getAuthPassword() !== ''
};

export const api = {
	getReleaseCatalog: (id: number, page = 1, signal?: AbortSignal) =>
		request<ReleaseCatalogResponse>(`/watchers/${id}/releases?page=${page}`, { signal }),
	downloadCatalogRelease: (id: number, releaseId: number, assetId: number) =>
		request<CatalogDownload>(`/watchers/${id}/releases/download`, {
			method: 'POST',
			body: JSON.stringify({ release_id: releaseId, asset_id: assetId })
		}),
	selectCatalogCandidate: (id: number, catalogId: number) =>
		request<{ message: string; version: string; catalog_id: number }>(
			`/watchers/${id}/releases/${catalogId}/candidate`,
			{ method: 'POST' }
		),
	deployCatalogRelease: (id: number, catalogId: number) =>
		request<{ message: string; deploy_log_id: number; version: string; catalog_id: number }>(
			`/watchers/${id}/releases/${catalogId}/deploy`,
			{ method: 'POST' }
		),
	// Auth
	authBootstrap: () => request<AuthBootstrapResponse>('/auth/bootstrap'),
	authLogin: (password: string) =>
		request<AuthStatusResponse>('/auth/login', {
			method: 'POST',
			body: JSON.stringify({ password })
		}),
	authStatus: () => request<AuthStatusResponse>('/auth/status'),
	updateAuthPassword: (currentPassword: string, newPassword: string) =>
		request<{ message: string; using_default_password: boolean }>('/auth/password', {
			method: 'PUT',
			body: JSON.stringify({ current_password: currentPassword, new_password: newPassword })
		}),

	// System
	status: () => request<SystemStatus>('/status'),
	agentLogs: (lines = 100, source: AgentLogSource = 'agent', signal?: AbortSignal, traceId = '') =>
		request<LogFileResponse>(
			`/logs?lines=${lines}&source=${source}&trace_id=${encodeURIComponent(traceId)}`,
			{ signal }
		),
	watcherLogs: (id: number, lines = 100, signal?: AbortSignal, traceId = '') =>
		request<LogFileResponse>(
			`/watchers/${id}/logs?lines=${lines}&trace_id=${encodeURIComponent(traceId)}`,
			{ signal }
		),

	// Watchers
	listWatchers: (signal?: AbortSignal) => request<Watcher[]>('/watchers', { signal }),
	getWatcher: (id: number, signal?: AbortSignal) => request<Watcher>(`/watchers/${id}`, { signal }),
	createWatcher: (data: WatcherWritePayload) =>
		request<Watcher>('/watchers', { method: 'POST', body: JSON.stringify(data) }),
	updateWatcher: (id: number, data: WatcherWritePayload) =>
		request<Watcher>(`/watchers/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
	deleteWatcher: (id: number) =>
		request<{ message: string }>(`/watchers/${id}`, { method: 'DELETE' }),
	triggerCheck: (id: number) =>
		request<{ message: string; request_id: string; poll_id: string; correlation_id: string }>(
			`/watchers/${id}/check`,
			{ method: 'POST' }
		),
	redeployWatcher: (id: number) =>
		request<{ message: string; deploy_log_id: number; log_url: string }>(
			`/watchers/${id}/redeploy`,
			{
				method: 'POST'
			}
		),
	watcherDeploys: (id: number, page = 1, pageSize = 10) =>
		request<PaginatedResponse<DeployLog>>(
			`/watchers/${id}/deploys?page=${page}&pageSize=${pageSize}`
		),
	watcherDeployLog: (id: number, logId: number) =>
		request<DeployLog>(`/watchers/${id}/deploys/${logId}`),
	watcherVersionCatalog: (id: number) =>
		request<{ versions: ReleaseInfo[]; downloads: CatalogDownload[]; current_version: string }>(
			`/watchers/${id}/versions`
		),
	watcherVersions: async (id: number) => {
		const res = await request<{ versions: ReleaseInfo[]; current_version: string }>(
			`/watchers/${id}/versions`
		);
		const current = (res.current_version || '').trim();
		if (!current) return res.versions ?? [];
		return (res.versions ?? []).map((v) => ({
			...v,
			is_current: v.version === current
		}));
	},
	rollbackWatcher: (id: number, version: string, reportGithub = true) =>
		request<RollbackResponse>(`/watchers/${id}/rollback`, {
			method: 'POST',
			body: JSON.stringify({ version, report_github: reportGithub })
		}),
	resumeWatcherUpdates: (id: number) =>
		request<{ message: string }>(`/watchers/${id}/resume`, { method: 'POST' }),
	deleteWatcherVersion: (id: number, version: string) =>
		request<{ message: string }>(`/watchers/${id}/versions/${encodeURIComponent(version)}`, {
			method: 'DELETE'
		}),
	watcherPolls: (id: number, page = 1, pageSize = 10, status = 'all') =>
		request<{ data: PollEvent[]; total: number; page: number; pageSize: number }>(
			`/watchers/${id}/polls?page=${page}&pageSize=${pageSize}&status=${status}`
		),
	watcherWebhookEvents: (id: number) => request<WebhookEvent[]>(`/watchers/${id}/webhook-events`),
	watcherWebhookDeliveries: async (id: number, page = 1, pageSize = 20) =>
		normalizePaginatedResponse(
			await request<PaginatedResponse<WebhookDelivery>>(
				`/watchers/${id}/webhook-deliveries?page=${page}&pageSize=${pageSize}`
			)
		),
	watcherWebhookDelivery: (id: number, deliveryId: number) =>
		request<WebhookDeliveryDetails>(`/watchers/${id}/webhook-deliveries/${deliveryId}`),
	sendWatcherWebhookTest: (id: number) =>
		request<{ message: string }>(`/watchers/${id}/webhook/test`, { method: 'POST' }),
	resumeWatcherWebhook: (id: number, replaySuppressed = false) =>
		request<{ message: string; replay_suppressed: boolean }>(`/watchers/${id}/webhook/resume`, {
			method: 'POST',
			body: JSON.stringify({ replay_suppressed: replaySuppressed })
		}),
	streamWatcherEvents: (
		id: number,
		onMessage: (data: string) => void | Promise<void>,
		onError?: (error: unknown) => void
	) =>
		openAuthenticatedEventStream(
			`/watchers/${id}/events`,
			{ onMessage, onError },
			{ reconnect: true }
		),
	streamDeployLog: (
		id: number,
		logId: number,
		onMessage: (data: string) => void | Promise<void>,
		onError?: (error: unknown) => void
	) =>
		openAuthenticatedEventStream(`/watchers/${id}/deploys/${logId}/stream`, { onMessage, onError }),
	// Services (flat)
	listServices: () => request<ServiceWithWatcher[]>('/services'),
	getService: (id: number) => request<{ service: Service; watcher: Watcher }>(`/services/${id}`),
	serviceStatus: (id: number) => request<Service>(`/services/${id}/status`),
	startService: (id: number) =>
		request<{ message: string }>(`/services/${id}/start`, { method: 'POST' }),
	stopService: (id: number) =>
		request<{ message: string }>(`/services/${id}/stop`, { method: 'POST' }),
	restartService: (id: number) =>
		request<{ message: string }>(`/services/${id}/restart`, { method: 'POST' }),
	serviceHealth: (id: number) =>
		request<{ status: string; http_status: number; error: string }>(`/services/${id}/health`),
	healthHistory: (id: number, limit = 50) =>
		request<HealthEvent[]>(`/services/${id}/health/history?limit=${limit}`),
	serviceLogs: (id: number, lines = 100, type = 'out') =>
		request<{ lines: string[] }>(`/services/${id}/logs?lines=${lines}&type=${type}`),
	serviceDeploys: (id: number, page = 1, pageSize = 10) =>
		request<PaginatedResponse<DeployLog>>(
			`/services/${id}/deploys?page=${page}&pageSize=${pageSize}`
		),
	syncServiceEnv: (id: number, envContent: string) =>
		request<{ message: string }>(`/services/${id}/env`, {
			method: 'PUT',
			body: JSON.stringify({ env_content: envContent })
		}),

	// Services (nested under watcher)
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
		}),

	// GitHub Integration
	inspectRepo: (repoUrl: string, releaseRef = 'latest', githubToken = '') =>
		request<InspectRepoResponse>('/github/inspect', {
			method: 'POST',
			body: JSON.stringify({
				repo_url: repoUrl,
				release_ref: releaseRef,
				github_token: githubToken
			})
		}),

	// Agent Self-Management
	selfVersion: () => request<SelfVersionResponse>('/self/version'),
	selfConfig: () => request<SelfConfigResponse>('/self/config'),
	updateSelfConfig: (data: UpdateSelfConfigRequest) =>
		request<{ message: string; notes: string[]; config: SelfConfigResponse }>('/self/config', {
			method: 'PUT',
			body: JSON.stringify(data)
		}),
	selfUpdateCheck: () => request<SelfUpdateCheckResponse>('/self/update-check'),
	selfUpdate: () => request<{ message: string }>('/self/update', { method: 'POST' }),
	selfRestart: () =>
		request<{ message: string; service_name: string }>('/self/restart', { method: 'POST' }),
	selfUninstall: () => request<{ script: string }>('/self/uninstall', { method: 'POST' }),

	interceptWatcher: async (id: number, intercept: boolean) =>
		request(`/watchers/${id}/intercept`, {
			method: 'POST',
			body: JSON.stringify({ intercept_next_release: intercept })
		}),
	approveRelease: async (id: number, version: string) =>
		request(`/watchers/${id}/approve`, { method: 'POST', body: JSON.stringify({ version }) }),
	getServiceConfigRevisions: async (id: number) =>
		request<{ data: ServiceConfigRevision[] }>(`/services/${id}/revisions`),
	updateServiceConfigRevision: async (
		id: number,
		target: string,
		envContent: string,
		configFiles?: ServiceConfigFile[]
	) =>
		request(`/services/${id}/revisions/${encodeURIComponent(target)}`, {
			method: 'PUT',
			body: JSON.stringify({ env_content: envContent, config_files: configFiles })
		}),
	deleteServiceConfigRevision: async (id: number, target: string) =>
		request(`/services/${id}/revisions/${encodeURIComponent(target)}`, { method: 'DELETE' }),
	getServiceSnapshotEnv: async (id: number, version: string) =>
		request<{ env_content: string }>(
			`/services/${id}/snapshots/${encodeURIComponent(version)}/env`
		),
	getWatcherConfigSnapshot: async (id: number, version: string, signal?: AbortSignal) =>
		request<ConfigSnapshot>(`/watchers/${id}/snapshots/${encodeURIComponent(version)}/config`, {
			signal
		}),

	discardRelease: async (id: number) =>
		request<{ message: string }>(`/watchers/${id}/discard`, { method: 'POST' }),
	getWatcherCandidate: async (id: number, target?: string) => {
		const q = target ? `?target=${encodeURIComponent(target)}` : '';
		return request<WatcherCandidateResponse>(`/watchers/${id}/candidate${q}`);
	},
	updateWatcherCandidate: async (
		id: number,
		targetVersion: string,
		services: { service_id: number; env_content: string; config_files?: ServiceConfigFile[] }[]
	) =>
		request<{ message: string }>(`/watchers/${id}/candidate`, {
			method: 'PUT',
			body: JSON.stringify({
				target_version: targetVersion,
				services
			})
		})
};
