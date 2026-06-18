<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type SystemStatus, type Watcher } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Alert from '$lib/components/ui/alert';
	import * as Button from '$lib/components/ui/button';
	import Badge from '$lib/components/ui/badge/badge.svelte';
	import {
		Activity,
		Clock,
		Eye,
		Server,
		Rocket,
		AlertCircle,
		Download,
		X,
		BellRing
	} from '@lucide/svelte';
	import { resolve } from '$app/paths';
	import { statusColor, timeAgo } from '$lib/utils';
	import {
		dismissSelfUpdate,
		getSelfUpdateSnapshot,
		lookupSelfUpdate,
		subscribeSelfUpdate
	} from '$lib/self-update';
	import type { SelfUpdateCheckResponse } from '$lib/api';

	let status = $state<SystemStatus | null>(null);
	let watchers = $state<Watcher[]>([]);
	let error = $state('');
	let selfUpdateInfo = $state<SelfUpdateCheckResponse | null>(null);
	let showSelfUpdateAlert = $state(false);

	function syncSelfUpdateState() {
		const snapshot = getSelfUpdateSnapshot();
		selfUpdateInfo = snapshot.info;
		showSelfUpdateAlert = snapshot.shouldNotify;
	}

	onMount(() => {
		syncSelfUpdateState();
		const unsubscribe = subscribeSelfUpdate((snapshot) => {
			selfUpdateInfo = snapshot.info;
			showSelfUpdateAlert = snapshot.shouldNotify;
		});

		const init = async () => {
			try {
				[status, watchers] = await Promise.all([api.status(), api.listWatchers()]);
				void lookupSelfUpdate({ silent: true });
			} catch (e) {
				error = e instanceof Error ? e.message : 'Failed to connect to API';
			}
		};

		void init();

		return unsubscribe;
	});

	function dismissUpdateAlert() {
		if (!selfUpdateInfo?.latest_version) return;
		dismissSelfUpdate(selfUpdateInfo.latest_version);
	}
</script>

<div class="space-y-8">
	<!-- Header -->
	<div>
		<h1 class="text-2xl font-bold tracking-tight">Dashboard</h1>
		<p class="text-sm text-muted-foreground">Watcher Agent overview</p>
	</div>

	{#if showSelfUpdateAlert && selfUpdateInfo?.update_available}
		<Alert.Root class="border-blue-500/30 bg-blue-500/10 text-blue-50">
			<BellRing class="h-4 w-4 text-blue-300" />
			<Alert.Title class="text-blue-200">New watcher version available</Alert.Title>
			<Alert.Description class="text-blue-50/85">
				Watcher {selfUpdateInfo.latest_version} is available. This agent is currently running {selfUpdateInfo.current_version}.
			</Alert.Description>
			<Alert.Action class="flex items-center gap-2">
				<a href={resolve('/settings')}>
					<Button.Root size="sm" class="bg-blue-600 text-white hover:bg-blue-700">
						<Download class="mr-2 h-4 w-4" />
						Update
					</Button.Root>
				</a>
				<Button.Root
					size="icon"
					variant="outline"
					class="h-8 w-8 border-blue-400/30 bg-transparent text-blue-100 hover:bg-blue-500/10 hover:text-white"
					onclick={dismissUpdateAlert}
				>
					<X class="h-4 w-4" />
				</Button.Root>
			</Alert.Action>
		</Alert.Root>
	{/if}

	{#if error}
		<div class="rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-400">
			<div class="flex items-center gap-2">
				<AlertCircle class="h-4 w-4" />
				<span>API connection failed: {error}</span>
			</div>
		</div>
	{/if}

	<!-- Stats grid -->
	{#if status}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
			<Card.Root class="border-border bg-card">
				<Card.Header class="flex flex-row items-center justify-between pb-2">
					<Card.Title class="text-sm font-medium text-muted-foreground">Status</Card.Title>
					<Activity class="h-4 w-4 text-emerald-400" />
				</Card.Header>
				<Card.Content>
					<div class="text-2xl font-bold capitalize">{status.status}</div>
					<p class="text-xs text-muted-foreground">{status.version} • {status.uptime_human}</p>
				</Card.Content>
			</Card.Root>

			<Card.Root class="border-border bg-card">
				<Card.Header class="flex flex-row items-center justify-between pb-2">
					<Card.Title class="text-sm font-medium text-muted-foreground">Watchers</Card.Title>
					<Eye class="h-4 w-4 text-blue-400" />
				</Card.Header>
				<Card.Content>
					<div class="text-2xl font-bold">{status.watcher_count}</div>
					<p class="text-xs text-muted-foreground">active poll loops</p>
				</Card.Content>
			</Card.Root>

			<Card.Root class="border-border bg-card">
				<Card.Header class="flex flex-row items-center justify-between pb-2">
					<Card.Title class="text-sm font-medium text-muted-foreground">Services</Card.Title>
					<Server class="h-4 w-4 text-violet-400" />
				</Card.Header>
				<Card.Content>
					<div class="text-2xl font-bold">{status.service_count}</div>
					<p class="text-xs text-muted-foreground">managed by NSSM</p>
				</Card.Content>
			</Card.Root>

			<Card.Root class="border-border bg-card">
				<Card.Header class="flex flex-row items-center justify-between pb-2">
					<Card.Title class="text-sm font-medium text-muted-foreground">Deploys (24h)</Card.Title>
					<Rocket class="h-4 w-4 text-amber-400" />
				</Card.Header>
				<Card.Content>
					<div class="text-2xl font-bold">{status.deploys_24h}</div>
					<p class="text-xs text-muted-foreground">in the last 24 hours</p>
				</Card.Content>
			</Card.Root>
		</div>
	{/if}

	<!-- Watcher status cards -->
	{#if watchers.length > 0}
		<div>
			<h2 class="mb-4 text-lg font-semibold">Watchers</h2>
			<div class="grid gap-4 sm:grid-cols-2">
				{#each watchers as w (w.id)}
					<a
						href={resolve(`/watchers/${w.id}`)}
						class="block transition-transform hover:scale-[1.01]"
					>
						<Card.Root class="border-border bg-card hover:border-muted-foreground/30">
							<Card.Header class="pb-3">
								<div class="flex items-center justify-between">
									<Card.Title class="text-base font-medium">{w.name}</Card.Title>
									<span
										class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium capitalize {statusColor(
											w.status
										)}"
									>
										{w.status}
									</span>
								</div>
								<p class="font-mono text-xs text-muted-foreground">{w.service_name}</p>
							</Card.Header>
							<Card.Content>
								<div class="flex items-center justify-between text-sm">
									<div class="flex items-center gap-1.5 text-muted-foreground">
										<Rocket class="h-3.5 w-3.5" />
										<span class="font-mono">{w.current_version || '—'}</span>
									</div>
									<div class="flex items-center gap-1.5 text-muted-foreground">
										<Clock class="h-3.5 w-3.5" />
										<span>{w.last_checked ? timeAgo(w.last_checked) : 'never'}</span>
									</div>
								</div>
								{#if w.last_error}
									<p class="mt-2 truncate text-xs text-red-400">{w.last_error}</p>
								{/if}
								<div class="mt-2 text-xs text-muted-foreground">
									{w.services.length} service{w.services.length !== 1 ? 's' : ''} • every {w.check_interval_sec}s
								</div>
							</Card.Content>
						</Card.Root>
					</a>
				{/each}
			</div>
		</div>
	{:else if !error}
		<Card.Root class="border-dashed border-border bg-card">
			<Card.Content class="flex flex-col items-center justify-center py-12 text-center">
				<Eye class="mb-3 h-10 w-10 text-muted-foreground/40" />
				<h3 class="text-sm font-medium text-muted-foreground">No watchers configured</h3>
				<p class="mt-1 text-xs text-muted-foreground/60">
					Add a watcher via the API or the Watchers page
				</p>
			</Card.Content>
		</Card.Root>
	{/if}
</div>
