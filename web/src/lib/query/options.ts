import { queryOptions } from '@tanstack/svelte-query';
import { api, type AgentLogSource } from '$lib/api';
import { keepWithin } from './client';

// Hierarchies make invalidation explicit; all server-side parameters belong in keys.
import { keys } from './keys';
export { keys } from './keys';

export const queries = {
	auth: () =>
		queryOptions({
			queryKey: [...keys.auth, 'status'],
			queryFn: ({ signal }) => api.authStatus(signal)
		}),
	bootstrap: () =>
		queryOptions({
			queryKey: [...keys.auth, 'bootstrap'],
			queryFn: ({ signal }) => api.authBootstrap(signal)
		}),
	status: () =>
		queryOptions({ queryKey: keys.status, queryFn: ({ signal }) => api.status(signal) }),
	watchers: () =>
		queryOptions({ queryKey: keys.watchers, queryFn: ({ signal }) => api.listWatchers(signal) }),
	watcher: (id: number) =>
		queryOptions({
			queryKey: [...keys.watcher(id), 'detail'],
			queryFn: ({ signal }) => api.getWatcher(id, signal)
		}),
	deploys: (id: number, page = 1, size = 10) =>
		queryOptions({
			queryKey: [...keys.watcher(id), 'deploys', page, size],
			queryFn: ({ signal }) => api.watcherDeploys(id, page, size, signal),
			placeholderData: keepWithin([...keys.watcher(id), 'deploys'])
		}),
	attempt: (id: number, attempt: number) =>
		queryOptions({
			queryKey: [...keys.watcher(id), 'attempt', attempt],
			queryFn: ({ signal }) => api.watcherDeployLog(id, attempt, signal)
		}),
	versions: (id: number) =>
		queryOptions({
			queryKey: [...keys.watcher(id), 'versions'],
			queryFn: ({ signal }) => api.watcherVersionCatalog(id, signal)
		}),
	polls: (id: number, page = 1, size = 10, status = 'all') =>
		queryOptions({
			queryKey: keys.polls(id, page, size, status),
			queryFn: ({ signal }) => api.watcherPolls(id, page, size, status, signal),
			placeholderData: keepWithin([...keys.watcher(id), 'polls', status])
		}),
	deliveries: (id: number, page = 1, size = 20) =>
		queryOptions({
			queryKey: [...keys.watcher(id), 'deliveries', page, size],
			queryFn: ({ signal }) => api.watcherWebhookDeliveries(id, page, size, signal),
			placeholderData: keepWithin([...keys.watcher(id), 'deliveries'])
		}),
	delivery: (id: number, delivery: number) =>
		queryOptions({
			queryKey: [...keys.watcher(id), 'delivery', delivery],
			queryFn: ({ signal }) => api.watcherWebhookDelivery(id, delivery, signal)
		}),
	releases: (id: number, page = 1) =>
		queryOptions({
			queryKey: [...keys.watcher(id), 'releases', page],
			queryFn: ({ signal }) => api.getReleaseCatalog(id, page, signal),
			placeholderData: keepWithin([...keys.watcher(id), 'releases'])
		}),
	candidate: (id: number, target: string, selection: string) =>
		queryOptions({
			queryKey: keys.candidate(id, target, selection),
			queryFn: ({ signal }) => api.getWatcherCandidate(id, target, signal)
		}),
	snapshot: (id: number, version: string) =>
		queryOptions({
			queryKey: keys.snapshot(id, version),
			queryFn: ({ signal }) => api.getWatcherConfigSnapshot(id, version, signal)
		}),
	services: () =>
		queryOptions({ queryKey: keys.services, queryFn: ({ signal }) => api.listServices(signal) }),
	service: (id: number) =>
		queryOptions({
			queryKey: [...keys.service(id), 'detail'],
			queryFn: ({ signal }) => api.getService(id, signal)
		}),
	serviceStatus: (id: number) =>
		queryOptions({
			queryKey: [...keys.service(id), 'status'],
			queryFn: ({ signal }) => api.serviceStatus(id, signal)
		}),
	health: (id: number, limit = 50) =>
		queryOptions({
			queryKey: [...keys.service(id), 'health', limit],
			queryFn: ({ signal }) => api.healthHistory(id, limit, signal)
		}),
	serviceDeploys: (id: number, page = 1, size = 10) =>
		queryOptions({
			queryKey: [...keys.service(id), 'deploys', page, size],
			queryFn: ({ signal }) => api.serviceDeploys(id, page, size, signal),
			placeholderData: keepWithin([...keys.service(id), 'deploys'])
		}),
	serviceLogs: (id: number, lines = 100, type = 'out') =>
		queryOptions({
			queryKey: [...keys.service(id), 'logs', type, lines],
			queryFn: ({ signal }) => api.serviceLogs(id, lines, type, signal),
			placeholderData: keepWithin([...keys.service(id), 'logs', type])
		}),
	logs: (id: number | undefined, lines: number, source: AgentLogSource, trace: string) =>
		queryOptions({
			queryKey: keys.logs(id, lines, source, trace),
			queryFn: ({ signal }) =>
				id
					? api.watcherLogs(id, lines, signal, trace)
					: api.agentLogs(lines, source, signal, trace),
			placeholderData: keepWithin(keys.logs(id, lines, source, trace).slice(0, -1))
		}),
	selfVersion: () =>
		queryOptions({
			queryKey: [...keys.self, 'version'],
			queryFn: ({ signal }) => api.selfVersion(signal)
		}),
	selfConfig: () =>
		queryOptions({
			queryKey: [...keys.self, 'config'],
			queryFn: ({ signal }) => api.selfConfig(signal)
		}),
	githubQuota: () =>
		queryOptions({
			queryKey: [...keys.self, 'github-quota'],
			queryFn: ({ signal }) => api.githubRateLimits(signal),
			refetchInterval: 30_000
		}),
	selfUpdate: () =>
		queryOptions({
			queryKey: [...keys.self, 'update-check'],
			queryFn: ({ signal }) => api.selfUpdateCheck(signal),
			staleTime: 30 * 60 * 1000
		})
};
