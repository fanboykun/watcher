export const keys = {
	auth: ['auth'] as const,
	status: ['system', 'status'] as const,
	watchers: ['watchers'] as const,
	watcher: (id: number) => ['watcher', id] as const,
	services: ['services'] as const,
	service: (id: number) => ['service', id] as const,
	self: ['self'] as const,
	candidate: (id: number, target: string, selection: string) =>
		['watcher', id, 'candidate', target, selection] as const,
	polls: (id: number, page: number, size: number, status: string) =>
		['watcher', id, 'polls', page, size, status] as const,
	snapshot: (id: number, version: string) => ['watcher', id, 'snapshot', version] as const,
	logs: (id: number | undefined, lines: number, source: string, trace: string) =>
		['logs', id ?? 'agent', lines, source, trace] as const
};
