import { createQuery, createMutation, useQueryClient } from '@tanstack/svelte-query';
import { queries, keys } from '$lib/query/options';
import { invalidate, message, serviceScope } from '$lib/query/client';
import { isIISService } from '$lib/api';
export function createServiceList() {
	const client = useQueryClient();
	const list = createQuery(() => queries.services());
	const services = $derived(list.data ?? []);
	const actionMutation = createMutation(() => ({
		mutationFn: async (v: {
			id: number;
			action: 'start' | 'stop' | 'restart' | 'health';
			run: () => Promise<{ message: string }>;
		}) => v.run(),
		onSuccess: (_data, v) => {
			const owner = services.find((service) => service.id === v.id)?.watcher_id;
			return invalidate(client, ...(owner ? serviceScope(v.id, owner) : [keys.service(v.id)]));
		}
	}));
	const ui = $state({
		query: '',
		runtime: 'all',
		actionMsg: '',
		actionError: '',
		serviceConfirmation: null as { id: number; name: string; action: 'stop' | 'restart' } | null
	});

	const filteredServices = $derived(
		services.filter(
			(service) =>
				[
					service.windows_service_name,
					service.watcher_name,
					service.binary_name,
					service.iis_site_name
				]
					.join(' ')
					.toLowerCase()
					.includes(ui.query.trim().toLowerCase()) &&
				(ui.runtime === 'all' ||
					(ui.runtime === 'iis'
						? isIISService(service.service_type)
						: !isIISService(service.service_type)))
		)
	);
	// Without any rows a failed read blocks the page; with rows it is only a stale warning.
	const blocking = $derived(list.isError && !list.data ? list.error : null);
	const error = $derived(blocking ? '' : message(list.error));
	const loading = $derived(list.isPending);

	const pendingServiceID = $derived(
		actionMutation.isPending ? (actionMutation.variables?.id ?? null) : null
	);
	const pendingServiceAction = $derived(
		actionMutation.isPending ? (actionMutation.variables?.action ?? null) : null
	);
	const pendingLabels = {
		start: 'Starting…',
		stop: 'Stopping…',
		restart: 'Restarting…',
		health: 'Checking…'
	};

	async function load() {
		await list.refetch();
	}

	async function serviceAction(
		id: number,
		action: 'start' | 'stop' | 'restart' | 'health',
		fn: () => Promise<{ message: string }>
	) {
		if (pendingServiceID !== null) return;
		ui.actionError = '';
		ui.actionMsg = '';
		try {
			const res = await actionMutation.mutateAsync({ id, action, run: fn });
			ui.actionMsg = `${services.find((service) => service.id === id)?.windows_service_name || 'Service'}: ${res.message}`;
			setTimeout(() => (ui.actionMsg = ''), 3000);
		} catch (e) {
			ui.actionError = e instanceof Error ? e.message : 'Action failed';
		}
	}
	return {
		ui,
		get pendingServiceID() {
			return pendingServiceID;
		},
		get pendingServiceAction() {
			return pendingServiceAction;
		},
		get pendingLabels() {
			return pendingLabels;
		},
		serviceAction,
		get client() {
			return client;
		},
		load,
		get loading() {
			return loading;
		},
		get services() {
			return services;
		},
		get filteredServices() {
			return filteredServices;
		},
		get error() {
			return error;
		},
		get blocking() {
			return blocking;
		}
	};
}
