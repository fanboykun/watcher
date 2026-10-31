import type {
	LogFileResponse,
	FeedAuthDraft,
	InspectRepoResponse,
	ReleaseSourceType,
	Watcher,
	WatcherWritePayload,
	DeployLog,
	RollbackResponse,
	PollEvent,
	PaginatedResponse,
	ReleaseInfo
} from './types';
import { request } from './transport';
export const watchersApi = {
	watcherLogs: (id: number, lines = 100, signal?: AbortSignal, traceId = '') =>
		request<LogFileResponse>(
			`/watchers/${id}/logs?lines=${lines}&trace_id=${encodeURIComponent(traceId)}`,
			{ signal }
		),
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
	watcherDeploys: (id: number, page = 1, pageSize = 10, signal?: AbortSignal) =>
		request<PaginatedResponse<DeployLog>>(
			`/watchers/${id}/deploys?page=${page}&pageSize=${pageSize}`,
			{ signal }
		),
	watcherDeployLog: (id: number, logId: number, signal?: AbortSignal) =>
		request<DeployLog>(`/watchers/${id}/deploys/${logId}`, { signal }),
	watcherVersions: async (id: number, signal?: AbortSignal) => {
		const res = await request<{ versions: ReleaseInfo[]; current_version: string }>(
			`/watchers/${id}/versions`,
			{ signal }
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
	watcherPolls: (id: number, page = 1, pageSize = 10, status = 'all', signal?: AbortSignal) =>
		request<{ data: PollEvent[]; total: number; page: number; pageSize: number }>(
			`/watchers/${id}/polls?page=${page}&pageSize=${pageSize}&status=${status}`,
			{ signal }
		),
	inspectRepo: (
		repoUrl: string,
		releaseRef = 'latest',
		githubToken = '',
		sourceType: ReleaseSourceType = 'github',
		feedAuth?: FeedAuthDraft
	) =>
		request<InspectRepoResponse>('/sources/inspect', {
			method: 'POST',
			body: JSON.stringify({
				source_type: sourceType,
				repo_url: repoUrl,
				feed_auth_type: feedAuth?.type ?? '',
				feed_auth_username: feedAuth?.username ?? '',
				feed_auth_header: feedAuth?.header ?? '',
				feed_auth_region: feedAuth?.region ?? '',
				feed_auth_secret: feedAuth?.secret ?? '',
				release_ref: releaseRef,
				github_token: githubToken
			})
		})
};
