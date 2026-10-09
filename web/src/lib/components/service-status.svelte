<script lang="ts">
	import { isIISService, type Service } from '$lib/api';
	import { Activity, Server, RefreshCw } from '@lucide/svelte';
	import * as Button from '$lib/components/ui/button';

	let {
		service,
		variant = 'compact',
		checking = false,
		onRefresh,
		refreshDisabled = false
	}: {
		service: Service;
		variant?: 'compact' | 'detail';
		checking?: boolean;
		onRefresh?: () => void | Promise<void>;
		refreshDisabled?: boolean;
	} = $props();

	const labels: Record<string, string> = {
		running: 'Running',
		stopped: 'Stopped',
		paused: 'Paused',
		start_pending: 'Starting',
		stop_pending: 'Stopping',
		continue_pending: 'Resuming',
		pause_pending: 'Pausing',
		not_installed: 'Not installed',
		not_applicable: 'IIS managed',
		healthy: 'Healthy',
		unhealthy: 'Unhealthy',
		error: 'Unreachable',
		unknown: 'Unknown'
	};
	const runtime = $derived(
		isIISService(service.service_type) ? 'not_applicable' : service.last_service_status || 'unknown'
	);
	const health = $derived(service.last_health_status || 'unknown');
	const observations = $derived([
		{
			name: 'Runtime',
			status: runtime,
			checkedAt: service.last_service_checked_at,
			error: service.last_service_error,
			icon: Server
		},
		{
			name: 'Health',
			status: health,
			checkedAt: service.last_health_checked_at,
			error: service.last_health_error,
			icon: Activity
		}
	]);
	function tone(status: string) {
		if (status === 'running' || status === 'healthy')
			return 'border-emerald-500/20 bg-emerald-500/10 text-emerald-400';
		if (
			status === 'stopped' ||
			status === 'not_installed' ||
			status === 'unhealthy' ||
			status === 'error'
		)
			return 'border-red-500/20 bg-red-500/10 text-red-400';
		if (status.endsWith('_pending') || status === 'paused')
			return 'border-amber-500/20 bg-amber-500/10 text-amber-400';
		return 'border-border bg-muted/40 text-muted-foreground';
	}
	function checkedAt(value?: string) {
		return value
			? new Date(value).toLocaleString(undefined, {
					month: 'short',
					day: 'numeric',
					hour: '2-digit',
					minute: '2-digit',
					second: '2-digit'
				})
			: 'Not checked yet';
	}
</script>

{#if variant === 'detail'}
	<section
		class="overflow-hidden rounded-xl border border-border bg-card"
		aria-label="Service status"
		aria-busy={checking}
	>
		<div
			class="flex flex-wrap items-center justify-between gap-3 border-b border-border px-5 py-3.5"
		>
			<div class="flex items-center gap-3">
				<h2 class="text-sm font-semibold">Service status</h2>
				<span class="hidden text-xs text-muted-foreground sm:inline">Last known state</span>
			</div>
			{#if onRefresh}
				<Button.Root
					variant="ghost"
					size="sm"
					class="h-7 text-xs text-muted-foreground"
					disabled={checking || refreshDisabled}
					onclick={onRefresh}
				>
					<RefreshCw
						class={checking ? 'size-3.5 animate-spin motion-reduce:animate-none' : 'size-3.5'}
					/>
					{checking ? 'Checking…' : 'Refresh status'}
				</Button.Root>
			{/if}
		</div>
		<div
			class="grid divide-y divide-border sm:grid-cols-2 sm:divide-x sm:divide-y-0"
			aria-live="polite"
		>
			{#each observations as observation (observation.name)}
				<div class="min-w-0 space-y-3 px-5 py-4">
					<div class="flex items-center justify-between gap-3">
						<p class="flex items-center gap-2 text-xs text-muted-foreground">
							<observation.icon class="size-3.5" />{observation.name === 'Runtime'
								? 'Windows service'
								: 'HTTP health'}
						</p>
						{#if observation.name === 'Health' && service.last_health_http_status}
							<span
								class="rounded border border-border px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground tabular-nums"
								>HTTP {service.last_health_http_status}</span
							>
						{/if}
					</div>
					<div class="flex items-center gap-2.5">
						<span
							class={`size-2 shrink-0 rounded-full border ${tone(observation.status)}`}
							aria-hidden="true"
						></span>
						<p class="font-mono text-xl font-medium tracking-tight">
							{labels[observation.status] || 'Unknown'}
						</p>
					</div>
					<p class="text-[11px] text-muted-foreground tabular-nums">
						{#if observation.status === 'not_applicable'}Runtime managed by IIS{:else}{observation.checkedAt
								? 'Checked '
								: ''}{checkedAt(observation.checkedAt)}{/if}
					</p>
					{#if observation.error}<p
							class="line-clamp-2 text-xs break-words text-muted-foreground"
							title={observation.error}
						>
							{observation.error}
						</p>{/if}
				</div>
			{/each}
		</div>
	</section>
{:else}
	<div class="flex flex-wrap items-center gap-1.5" aria-label="Last known service status">
		{#each observations as observation (observation.name)}
			<span
				class={`inline-flex items-center gap-1.5 rounded-md border px-2 py-1 text-[11px] font-medium ${tone(observation.status)}`}
				title={`${observation.name} · ${checkedAt(observation.checkedAt)}${observation.error ? ` · ${observation.error}` : ''}`}
			>
				<observation.icon class="size-3" />
				<span class="sr-only">{observation.name}: </span>{labels[observation.status] || 'Unknown'}
			</span>
		{/each}
	</div>
{/if}
