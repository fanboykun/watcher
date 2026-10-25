import type {
	ServiceConfigFile,
	ServiceConfigRevision,
	ConfigSnapshot,
	WatcherCandidateResponse
} from './types';
import { request } from './transport';
export const configurationApi = {
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
	getWatcherCandidate: async (id: number, target?: string, signal?: AbortSignal) => {
		const q = target ? `?target=${encodeURIComponent(target)}` : '';
		return request<WatcherCandidateResponse>(`/watchers/${id}/candidate${q}`, { signal });
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
