import type {
	AgentLogSource,
	LogFileResponse,
	AuthStatusResponse,
	AuthBootstrapResponse,
	SystemStatus,
	SelfVersionResponse,
	SelfUpdateCheckResponse,
	SelfConfigResponse,
	UpdateSelfConfigRequest
} from './types';
import { request } from './transport';
export const systemApi = {
	authBootstrap: (signal?: AbortSignal) =>
		request<AuthBootstrapResponse>('/auth/bootstrap', { signal }),
	authLogin: (password: string) =>
		request<AuthStatusResponse>('/auth/login', {
			method: 'POST',
			body: JSON.stringify({ password })
		}),
	authStatus: (signal?: AbortSignal) => request<AuthStatusResponse>('/auth/status', { signal }),
	updateAuthPassword: (currentPassword: string, newPassword: string) =>
		request<{ message: string; using_default_password: boolean }>('/auth/password', {
			method: 'PUT',
			body: JSON.stringify({ current_password: currentPassword, new_password: newPassword })
		}),
	status: (signal?: AbortSignal) => request<SystemStatus>('/status', { signal }),
	agentLogs: (lines = 100, source: AgentLogSource = 'agent', signal?: AbortSignal, traceId = '') =>
		request<LogFileResponse>(
			`/logs?lines=${lines}&source=${source}&trace_id=${encodeURIComponent(traceId)}`,
			{ signal }
		),
	selfVersion: (signal?: AbortSignal) => request<SelfVersionResponse>('/self/version', { signal }),
	selfConfig: (signal?: AbortSignal) => request<SelfConfigResponse>('/self/config', { signal }),
	updateSelfConfig: (data: UpdateSelfConfigRequest) =>
		request<{ message: string; notes: string[]; config: SelfConfigResponse }>('/self/config', {
			method: 'PUT',
			body: JSON.stringify(data)
		}),
	selfUpdateCheck: (signal?: AbortSignal) =>
		request<SelfUpdateCheckResponse>('/self/update-check', { signal }),
	selfUpdate: () => request<{ message: string }>('/self/update', { method: 'POST' }),
	selfRestart: () =>
		request<{ message: string; service_name: string }>('/self/restart', { method: 'POST' }),
	selfUninstall: () => request<{ script: string }>('/self/uninstall', { method: 'POST' })
};
