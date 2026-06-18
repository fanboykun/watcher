<script lang="ts">
	import { resolve } from '$app/paths';
	import * as Card from '$lib/components/ui/card';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Table from '$lib/components/ui/table';
	import * as Button from '$lib/components/ui/button';
	import * as Select from '$lib/components/ui/select/index.js';
	import { api, type Watcher, type WebhookDelivery, type WebhookDeliveryDetails } from '$lib/api';
	import { formatDate, timeAgo } from '$lib/utils';
	import { webhookDocsHref } from '$lib/webhooks';
	import {
		Activity,
		AlertTriangle,
		CheckCircle2,
		Clock3,
		ExternalLink,
		KeyRound,
		Link2,
		PlayCircle,
		RotateCcw,
		Send,
		ShieldAlert,
		Siren
	} from '@lucide/svelte';

	let {
		watcher,
		deliveries,
		deliveryPage = $bindable(1),
		deliveryPageSize = $bindable(20),
		deliveryTotal,
		onPageChange,
		onPageSizeChange,
		onSendTest,
		onResume,
		onResumeReplay
	}: {
		watcher: Watcher;
		deliveries: WebhookDelivery[];
		deliveryPage: number;
		deliveryPageSize: number;
		deliveryTotal: number;
		onPageChange: (page: number) => void | Promise<void>;
		onPageSizeChange: (size: number) => void | Promise<void>;
		onSendTest: () => void | Promise<void>;
		onResume: () => void | Promise<void>;
		onResumeReplay: () => void | Promise<void>;
	} = $props();

	let showDeliveryDetails = $state(false);
	let selectedDeliveryDetails = $state<WebhookDeliveryDetails | null>(null);
	let selectedDeliveryLoading = $state(false);
	let selectedDeliveryError = $state('');

	type DeliverySummary = {
		successCount: number;
		failedCount: number;
		pendingCount: number;
		suppressedCount: number;
		lastSuccessful: WebhookDelivery | null;
		lastFailed: WebhookDelivery | null;
		nextRetry: WebhookDelivery | null;
	};

	function normalizeStatus(value: string | null | undefined) {
		return (value ?? '').trim().toLowerCase();
	}

	function isSucceeded(status: string) {
		return ['succeeded', 'success', 'delivered', 'ok'].includes(normalizeStatus(status));
	}

	function isFailed(status: string) {
		return ['failed', 'error', 'dead'].includes(normalizeStatus(status));
	}

	function isPending(status: string) {
		return ['pending', 'queued', 'retrying', 'in_progress'].includes(normalizeStatus(status));
	}

	function isSuppressed(status: string) {
		return ['suppressed', 'paused'].includes(normalizeStatus(status));
	}

	function statusTone(status: string) {
		if (isSucceeded(status)) return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-300';
		if (isFailed(status)) return 'border-red-500/30 bg-red-500/10 text-red-300';
		if (isSuppressed(status)) return 'border-amber-500/30 bg-amber-500/10 text-amber-300';
		return 'border-blue-500/30 bg-blue-500/10 text-blue-300';
	}

	function statusLabel(status: string) {
		const value = normalizeStatus(status);
		if (!value) return 'Unknown';
		return value.replace(/_/g, ' ');
	}

	function preview(text: string | null | undefined, max = 110) {
		const value = (text ?? '').trim();
		if (!value) return '—';
		return value.length <= max ? value : `${value.slice(0, max)}...`;
	}

	function prettyJson(value: unknown) {
		if (value === null || value === undefined) return '—';
		if (typeof value === 'string') return value;
		try {
			return JSON.stringify(value, null, 2);
		} catch {
			return String(value);
		}
	}

	function headerValue(value: string | number | null | undefined) {
		if (value === null || value === undefined || value === '') return '—';
		return String(value);
	}

	function routeSource(url: string) {
		return url ? 'Watcher override' : 'Global default';
	}

	function authSource(watcher: Watcher) {
		return watcher.has_webhook_signing_secret ? 'Watcher override secret' : 'Global default secret';
	}

	function authSourceHelp(watcher: Watcher) {
		if (watcher.has_webhook_signing_secret) {
			return 'Paste the exact same `whsec_...` value into your receiver and Watcher. The secret must start with `whsec_` and the masked text shown here is only a display hint.';
		}
		return 'Watcher will inherit the global `whsec_...` signing secret from system settings. The secret must start with `whsec_`, and your receiver must use the same exact value to verify signatures.';
	}

	const deliverySummary = $derived.by<DeliverySummary>(() => {
		let successCount = 0;
		let failedCount = 0;
		let pendingCount = 0;
		let suppressedCount = 0;
		let lastSuccessful: WebhookDelivery | null = null;
		let lastFailed: WebhookDelivery | null = null;
		let nextRetry: WebhookDelivery | null = null;

		for (const delivery of deliveries) {
			const status = normalizeStatus(delivery.status);
			if (isSucceeded(status)) {
				successCount += 1;
				if (!lastSuccessful) lastSuccessful = delivery;
			} else if (isFailed(status)) {
				failedCount += 1;
				if (!lastFailed) lastFailed = delivery;
			} else if (isSuppressed(status)) {
				suppressedCount += 1;
			} else if (isPending(status)) {
				pendingCount += 1;
			}

			if (!nextRetry && delivery.next_retry_at) {
				nextRetry = delivery;
			}
		}

		return {
			successCount,
			failedCount,
			pendingCount,
			suppressedCount,
			lastSuccessful,
			lastFailed,
			nextRetry
		};
	});

	async function openDeliveryDetails(delivery: WebhookDelivery) {
		selectedDeliveryLoading = true;
		selectedDeliveryError = '';
		selectedDeliveryDetails = null;
		showDeliveryDetails = true;

		try {
			selectedDeliveryDetails = await api.watcherWebhookDelivery(watcher.id, delivery.id);
		} catch (error) {
			selectedDeliveryError = error instanceof Error ? error.message : 'Unable to load delivery details';
		} finally {
			selectedDeliveryLoading = false;
		}
	}

	function closeDeliveryDetails() {
		showDeliveryDetails = false;
		selectedDeliveryDetails = null;
		selectedDeliveryLoading = false;
		selectedDeliveryError = '';
	}
</script>

<div class="space-y-5">
	<Card.Root class="overflow-hidden border-border bg-card">
		<div
			class="border-b border-border/70 bg-[linear-gradient(135deg,rgba(59,130,246,0.08),rgba(17,24,39,0))]"
		>
			<Card.Header class="gap-4">
				<div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
					<div class="space-y-2">
						<div
							class="inline-flex items-center gap-2 text-xs font-medium tracking-[0.18em] text-muted-foreground uppercase"
						>
							<Activity class="h-3.5 w-3.5" />
							Webhook operations
						</div>
						<div>
							<Card.Title class="text-base">Delivery Health And Recovery</Card.Title>
							<Card.Description class="mt-1 max-w-3xl">
								See whether this watcher can reach its receiver, what credentials it is using, and
								which recovery action is safe before you resume traffic.
							</Card.Description>
						</div>
					</div>
					<div class="flex flex-wrap gap-2">
						<a href={resolve('/webhooks')}>
							<Button.Root size="sm" variant="outline">
								<ExternalLink class="mr-2 h-4 w-4" />
								Webhook Hub
							</Button.Root>
						</a>
						<a href={resolve('/docs/webhooks')}>
							<Button.Root size="sm" variant="outline">Guide</Button.Root>
						</a>
						<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
						<a href={webhookDocsHref} target="_blank" rel="noopener noreferrer">
							<Button.Root size="sm" variant="outline">Repo Docs</Button.Root>
						</a>
					</div>
				</div>
			</Card.Header>
		</div>

		<Card.Content class="space-y-5 pt-5">
			<div class="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
				<div class="rounded-xl border border-border/70 bg-muted/20 p-4">
					<p class="text-[11px] font-medium tracking-[0.16em] text-muted-foreground uppercase">
						Delivery state
					</p>
					<div class="mt-3 flex items-center gap-2">
						<span
							class={`inline-flex rounded-full border px-2.5 py-1 text-xs font-medium ${watcher.webhook_enabled ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-300' : 'border-zinc-500/30 bg-zinc-500/10 text-zinc-300'}`}
						>
							{watcher.webhook_enabled ? 'Enabled' : 'Disabled'}
						</span>
						<span
							class={`inline-flex rounded-full border px-2.5 py-1 text-xs font-medium ${watcher.webhook_paused_at ? 'border-amber-500/30 bg-amber-500/10 text-amber-300' : 'border-emerald-500/30 bg-emerald-500/10 text-emerald-300'}`}
						>
							{watcher.webhook_paused_at ? 'Paused' : 'Live'}
						</span>
					</div>
					<p class="mt-3 text-sm text-muted-foreground">
						{#if watcher.webhook_paused_at}
							Paused {timeAgo(watcher.webhook_paused_at)} after delivery failures.
						{:else if watcher.webhook_enabled}
							New webhook events can leave the outbox immediately.
						{:else}
							This watcher will not emit webhook events until delivery is enabled.
						{/if}
					</p>
				</div>

				<div class="rounded-xl border border-border/70 bg-muted/20 p-4">
					<p class="text-[11px] font-medium tracking-[0.16em] text-muted-foreground uppercase">
						Failure streak
					</p>
					<p class="mt-3 text-3xl font-semibold">{watcher.webhook_failure_streak}</p>
					<p class="mt-2 text-sm text-muted-foreground">
						Consecutive failed deliveries before the receiver started succeeding again.
					</p>
				</div>

				<div class="rounded-xl border border-border/70 bg-muted/20 p-4">
					<p class="text-[11px] font-medium tracking-[0.16em] text-muted-foreground uppercase">
						Recent successes
					</p>
					<p class="mt-3 text-3xl font-semibold">{deliverySummary.successCount}</p>
					<p class="mt-2 text-sm text-muted-foreground">
						{#if deliverySummary.lastSuccessful}
							Last success {timeAgo(
								deliverySummary.lastSuccessful.completed_at ||
									deliverySummary.lastSuccessful.last_attempt_at ||
									deliverySummary.lastSuccessful.created_at
							)}.
						{:else}
							No successful delivery in the currently loaded history slice.
						{/if}
					</p>
				</div>

				<div class="rounded-xl border border-border/70 bg-muted/20 p-4">
					<p class="text-[11px] font-medium tracking-[0.16em] text-muted-foreground uppercase">
						Pending recovery
					</p>
					<p class="mt-3 text-3xl font-semibold">
						{deliverySummary.pendingCount + deliverySummary.suppressedCount}
					</p>
					<p class="mt-2 text-sm text-muted-foreground">
						{#if deliverySummary.nextRetry}
							Retry queued for {formatDate(deliverySummary.nextRetry.next_retry_at)}.
						{:else if deliverySummary.suppressedCount > 0}
							Suppressed deliveries are waiting for an operator decision.
						{:else}
							No retry or suppressed backlog in the currently loaded history slice.
						{/if}
					</p>
				</div>
			</div>

			{#if watcher.webhook_paused_at || watcher.webhook_pause_reason}
				<div class="rounded-2xl border border-amber-500/30 bg-amber-500/10 p-4">
					<div class="flex items-start gap-3">
						<div class="rounded-full bg-amber-500/15 p-2 text-amber-300">
							<Siren class="h-4 w-4" />
						</div>
						<div class="min-w-0 flex-1">
							<p class="font-medium text-amber-200">
								Delivery is paused and needs an operator choice
							</p>
							<p class="mt-1 text-sm text-amber-100/90">
								Fix the receiver first, then choose whether only future events should continue or
								whether suppressed backlog should be replayed in FIFO order.
							</p>
							{#if watcher.webhook_pause_reason}
								<p
									class="mt-3 rounded-lg border border-amber-500/20 bg-black/10 px-3 py-2 text-sm text-amber-100/90"
								>
									{watcher.webhook_pause_reason}
								</p>
							{/if}
						</div>
					</div>
				</div>
			{/if}

			<div class="grid gap-4 xl:grid-cols-[1.1fr_0.9fr]">
				<div class="rounded-2xl border border-border/70 bg-muted/15 p-4">
					<div class="flex items-center gap-2">
						<Link2 class="h-4 w-4 text-blue-300" />
						<h3 class="font-medium">Routing and credentials</h3>
					</div>
					<div class="mt-4 grid gap-3 sm:grid-cols-2">
						<div class="min-w-0 rounded-xl border border-border/60 bg-background/40 p-3">
							<p class="text-xs tracking-[0.14em] text-muted-foreground uppercase">
								Resolved URL source
							</p>
							<p class="mt-2 text-sm font-medium">{routeSource(watcher.webhook_url)}</p>
							<p class="mt-1 break-all whitespace-normal font-mono text-xs text-muted-foreground">
								{watcher.webhook_url || 'Uses global default URL from system settings'}
							</p>
						</div>
						<div class="min-w-0 rounded-xl border border-border/60 bg-background/40 p-3">
							<div
								class="flex items-center gap-2 text-xs tracking-[0.14em] text-muted-foreground uppercase"
							>
								<KeyRound class="h-3.5 w-3.5" />
								Secret source
							</div>
							<p class="mt-2 text-sm font-medium">{authSource(watcher)}</p>
							<p class="mt-1 break-all whitespace-normal text-xs text-muted-foreground">
								{watcher.has_webhook_signing_secret
									? watcher.webhook_signing_secret_masked || 'Watcher signing secret configured'
									: 'Signing secret comes from the global webhook defaults'}
							</p>
							<p class="mt-2 text-xs text-muted-foreground">
								{authSourceHelp(watcher)}
							</p>
						</div>
						<div class="min-w-0 rounded-xl border border-border/60 bg-background/40 p-3">
							<p class="text-xs tracking-[0.14em] text-muted-foreground uppercase">
								Recent outcome
							</p>
							{#if deliverySummary.lastFailed}
								<p class="mt-2 text-sm font-medium text-red-300">Latest failure needs attention</p>
								<p class="mt-1 break-words whitespace-normal text-xs text-muted-foreground">
									{preview(
										deliverySummary.lastFailed.error || deliverySummary.lastFailed.response_body
									)}
								</p>
							{:else if deliverySummary.lastSuccessful}
								<p class="mt-2 text-sm font-medium text-emerald-300">Recent delivery succeeded</p>
								<p class="mt-1 break-words whitespace-normal text-xs text-muted-foreground">
									HTTP {deliverySummary.lastSuccessful.response_status_code || 'n/a'} on
									{deliverySummary.lastSuccessful.event_type}
								</p>
							{:else}
								<p class="mt-2 text-sm font-medium">No delivery history yet</p>
								<p class="mt-1 break-words whitespace-normal text-xs text-muted-foreground">
									Send a test webhook after endpoint and signing secret configuration are ready.
								</p>
							{/if}
						</div>
						<div class="rounded-xl border border-border/60 bg-background/40 p-3">
							<p class="text-xs tracking-[0.14em] text-muted-foreground uppercase">
								Operator guidance
							</p>
							{#if watcher.webhook_paused_at}
								<p class="mt-2 text-sm font-medium text-amber-300">Resume carefully</p>
								<p class="mt-1 break-words whitespace-normal text-xs text-muted-foreground">
									Use replay only if the downstream receiver can safely process delayed events.
								</p>
							{:else}
								<p class="mt-2 text-sm font-medium text-blue-300">Safe validation path</p>
								<p class="mt-1 break-words whitespace-normal text-xs text-muted-foreground">
									Send a synthetic test event to verify URL, auth, and receiver behavior end to end.
								</p>
							{/if}
						</div>
					</div>
				</div>

				<div class="rounded-2xl border border-border/70 bg-muted/15 p-4">
					<div class="flex items-center gap-2">
						<ShieldAlert class="h-4 w-4 text-blue-300" />
						<h3 class="font-medium">Recovery playbook</h3>
					</div>
					<div class="mt-4 space-y-3">
						<div class="min-w-0 rounded-xl border border-border/60 bg-background/40 p-3">
							<div class="flex items-center justify-between gap-3">
								<div>
									<p class="font-medium">1. Validate the receiver</p>
									<p class="mt-1 break-words whitespace-normal text-sm text-muted-foreground">
										Queues a synthetic <code>watcher.webhook_test</code> event through the same outbox
										and retry pipeline as real traffic.
									</p>
								</div>
								<Button.Root size="sm" variant="outline" onclick={onSendTest}>
									<Send class="mr-2 h-4 w-4" />
									Send Test
								</Button.Root>
							</div>
						</div>

						<div class="min-w-0 rounded-xl border border-border/60 bg-background/40 p-3">
							<div class="flex items-center justify-between gap-3">
								<div>
									<p class="font-medium">2. Resume future deliveries only</p>
									<p class="mt-1 break-words whitespace-normal text-sm text-muted-foreground">
										Clears the paused state and lets only new events continue. Use this when older
										suppressed events should stay frozen for manual review.
									</p>
								</div>
								<Button.Root size="sm" variant="outline" onclick={onResume}>
									<PlayCircle class="mr-2 h-4 w-4" />
									Resume Future Only
								</Button.Root>
							</div>
						</div>

						<div class="min-w-0 rounded-xl border border-amber-500/20 bg-amber-500/5 p-3">
							<div class="flex items-center justify-between gap-3">
								<div>
									<div
										class="inline-flex items-center gap-2 text-xs font-medium tracking-[0.14em] text-amber-300 uppercase"
									>
										<AlertTriangle class="h-3.5 w-3.5" />
										Use only when backlog replay is safe
									</div>
									<p class="mt-2 font-medium">3. Resume and replay suppressed events</p>
									<p class="mt-1 break-words whitespace-normal text-sm text-muted-foreground">
										Re-queues suppressed deliveries back to pending and replays them in normal FIFO
										order. Choose this only if your receiver is idempotent or can handle delayed
										duplicates safely.
									</p>
								</div>
								<Button.Root
									size="sm"
									class="bg-amber-600 text-white hover:bg-amber-700"
									onclick={onResumeReplay}
								>
									<RotateCcw class="mr-2 h-4 w-4" />
									Replay Suppressed
								</Button.Root>
							</div>
						</div>
					</div>
				</div>
			</div>
		</Card.Content>
	</Card.Root>

	<div class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
		<div>
			<h3 class="text-base font-semibold">Recent deliveries</h3>
			<p class="mt-1 text-sm text-muted-foreground">
				Each row shows the business event, transport outcome, and the next recovery hint.
			</p>
		</div>
		<div class="flex items-center gap-2">
			<div class="text-xs text-muted-foreground">
				Showing {deliveries.length === 0 ? 0 : (deliveryPage - 1) * deliveryPageSize + 1} -
				{Math.min(deliveryPage * deliveryPageSize, deliveryTotal)} of {deliveryTotal}
			</div>
			<Select.Root
				type="single"
				value={String(deliveryPageSize)}
				onValueChange={(v) => {
					if (v) {
						onPageSizeChange(Number(v));
					}
				}}
			>
				<Select.Trigger class="h-8 w-28 bg-card text-xs">
					{deliveryPageSize} / page
				</Select.Trigger>
				<Select.Content>
					<Select.Item value="20" label="20 / page">20 / page</Select.Item>
					<Select.Item value="50" label="50 / page">50 / page</Select.Item>
					<Select.Item value="100" label="100 / page">100 / page</Select.Item>
				</Select.Content>
			</Select.Root>
			<Button.Root
				variant="outline"
				size="sm"
				disabled={deliveryPage <= 1}
				onclick={() => onPageChange(deliveryPage - 1)}
			>
				Prev
			</Button.Root>
			<Button.Root
				variant="outline"
				size="sm"
				disabled={deliveryPage * deliveryPageSize >= deliveryTotal}
				onclick={() => onPageChange(deliveryPage + 1)}
			>
				Next
			</Button.Root>
		</div>
	</div>

	<Card.Root class="border-border bg-card">
		<Table.Root>
			<Table.Header>
				<Table.Row class="border-border hover:bg-transparent">
					<Table.Head>Event</Table.Head>
					<Table.Head>Outcome</Table.Head>
					<Table.Head>Route</Table.Head>
					<Table.Head>Activity</Table.Head>
					<Table.Head>Operator Note</Table.Head>
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#if deliveries.length === 0}
					<Table.Row class="border-border">
						<Table.Cell colspan={5} class="py-10 text-center">
							<div class="mx-auto max-w-md space-y-2">
								<p class="font-medium">No webhook deliveries yet</p>
								<p class="text-sm text-muted-foreground">
									Once this watcher emits webhook traffic, delivery attempts, retry state, and
									receiver responses will show up here.
								</p>
							</div>
						</Table.Cell>
					</Table.Row>
				{:else}
					{#each deliveries as delivery (delivery.id)}
						<Table.Row class="border-border align-top">
							<Table.Cell class="min-w-60">
								<div class="space-y-1">
									<p class="font-mono text-xs text-blue-200">{delivery.event_type || '—'}</p>
									<p class="break-words whitespace-normal text-sm">
										{delivery.summary || 'No summary recorded'}
									</p>
									<p class="text-xs text-muted-foreground">Delivery ID {delivery.delivery_id}</p>
								</div>
							</Table.Cell>
							<Table.Cell class="min-w-42">
								<div class="space-y-2">
									<span
										class={`inline-flex rounded-full border px-2.5 py-1 text-xs font-medium capitalize ${statusTone(delivery.status)}`}
									>
										{statusLabel(delivery.status)}
									</span>
									<div class="text-xs text-muted-foreground">
										<p>Attempt #{delivery.attempt_number}</p>
										<p>HTTP {delivery.response_status_code || '—'}</p>
									</div>
								</div>
							</Table.Cell>
							<Table.Cell class="min-w-52">
								<div class="space-y-1">
									<p class="break-all whitespace-normal font-mono text-xs">{delivery.resolved_url || '—'}</p>
									<p class="break-all whitespace-normal text-xs text-muted-foreground">
										Auth {delivery.auth_type || '—'} via {delivery.secret_source || '—'}
									</p>
								</div>
							</Table.Cell>
							<Table.Cell class="min-w-48">
								<div class="space-y-1 text-xs text-muted-foreground">
									<p>Last attempt {formatDate(delivery.last_attempt_at || delivery.created_at)}</p>
									{#if delivery.completed_at}
										<p>Completed {timeAgo(delivery.completed_at)}</p>
									{/if}
									{#if delivery.next_retry_at}
										<p class="text-blue-300">Next retry {formatDate(delivery.next_retry_at)}</p>
									{/if}
									{#if delivery.replayed_at}
										<p class="text-amber-300">Replayed {formatDate(delivery.replayed_at)}</p>
									{/if}
								</div>
							</Table.Cell>
							<Table.Cell class="max-w-80">
								<div class="space-y-1 text-xs">
									{#if delivery.error}
										<p class="break-words whitespace-normal text-red-300">{preview(delivery.error, 160)}</p>
									{:else if delivery.response_body}
										<p class="break-words whitespace-normal text-muted-foreground">
											{preview(delivery.response_body, 160)}
										</p>
									{:else if delivery.next_retry_at}
										<p class="text-blue-300">Waiting for retry window.</p>
									{:else if isSuppressed(delivery.status)}
										<p class="text-amber-300">Suppressed until an operator resumes delivery.</p>
									{:else if isSucceeded(delivery.status)}
										<p class="text-emerald-300">Receiver accepted this delivery.</p>
									{:else}
										<p class="text-muted-foreground">No receiver note recorded.</p>
									{/if}
									<div class="pt-1">
										<Button.Root
											variant="outline"
											size="sm"
											class="h-8 text-xs"
											onclick={() => openDeliveryDetails(delivery)}
										>
											View details
										</Button.Root>
									</div>
								</div>
							</Table.Cell>
						</Table.Row>
					{/each}
				{/if}
			</Table.Body>
		</Table.Root>
	</Card.Root>

	<Dialog.Root bind:open={showDeliveryDetails}>
		<Dialog.Content class="max-h-[90vh] w-[min(1100px,95vw)] overflow-hidden sm:max-w-[1100px]">
			<Dialog.Header>
				<Dialog.Title>Webhook Delivery Details</Dialog.Title>
				<Dialog.Description>
					Inspect the exact payload, signed request metadata, and receiver response for this
					delivery attempt.
				</Dialog.Description>
			</Dialog.Header>

			<div class="max-h-[72vh] space-y-4 overflow-y-auto pr-1">
				{#if selectedDeliveryLoading}
					<div class="rounded-xl border border-border/60 bg-muted/20 p-4 text-sm text-muted-foreground">
						Loading delivery details...
					</div>
				{:else if selectedDeliveryError}
					<div class="rounded-xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-200">
						{selectedDeliveryError}
					</div>
				{:else if selectedDeliveryDetails}
					<div class="grid gap-4 lg:grid-cols-2">
						<div class="space-y-4">
							<div class="rounded-xl border border-border/60 bg-background/40 p-4">
								<p class="text-xs tracking-[0.14em] text-muted-foreground uppercase">Delivery</p>
								<div class="mt-3 grid gap-2 text-sm">
									<p><span class="text-muted-foreground">Event:</span> {selectedDeliveryDetails.delivery.event_type}</p>
									<p><span class="text-muted-foreground">Delivery ID:</span> {selectedDeliveryDetails.delivery.delivery_id}</p>
									<p><span class="text-muted-foreground">Status:</span> {selectedDeliveryDetails.delivery.status}</p>
									<p><span class="text-muted-foreground">Attempt:</span> #{selectedDeliveryDetails.delivery.attempt_number}</p>
									<p><span class="text-muted-foreground">HTTP:</span> {headerValue(selectedDeliveryDetails.delivery.response_status_code)}</p>
									<p><span class="text-muted-foreground">Created:</span> {formatDate(selectedDeliveryDetails.delivery.created_at)}</p>
									<p><span class="text-muted-foreground">Last attempt:</span> {headerValue(selectedDeliveryDetails.delivery.last_attempt_at)}</p>
									<p><span class="text-muted-foreground">Completed:</span> {headerValue(selectedDeliveryDetails.delivery.completed_at)}</p>
								</div>
							</div>

							<div class="rounded-xl border border-border/60 bg-background/40 p-4">
								<p class="text-xs tracking-[0.14em] text-muted-foreground uppercase">Request</p>
								<div class="mt-3 space-y-3 text-sm">
									<p class="break-all font-mono text-xs text-muted-foreground">{selectedDeliveryDetails.request.url}</p>
									<p><span class="text-muted-foreground">Auth:</span> {selectedDeliveryDetails.request.auth_type}</p>
									<p><span class="text-muted-foreground">Secret source:</span> {selectedDeliveryDetails.request.secret_source}</p>
									<div class="space-y-1">
										<p class="text-xs tracking-[0.14em] text-muted-foreground uppercase">Headers</p>
										<pre class="overflow-x-auto rounded-lg border border-border/60 bg-muted/30 p-3 text-xs leading-5 text-muted-foreground">{prettyJson(selectedDeliveryDetails.request.headers)}</pre>
									</div>
								</div>
							</div>
						</div>

						<div class="space-y-4">
							<div class="rounded-xl border border-border/60 bg-background/40 p-4">
								<p class="text-xs tracking-[0.14em] text-muted-foreground uppercase">Event payload</p>
								<p class="mt-2 text-sm text-muted-foreground">
									This is the exact business event body we signed and sent to the receiver.
								</p>
								<pre class="mt-3 max-h-80 overflow-auto rounded-lg border border-border/60 bg-muted/30 p-3 text-xs leading-5 text-muted-foreground">{prettyJson(selectedDeliveryDetails.event.payload)}</pre>
							</div>

							<div class="rounded-xl border border-border/60 bg-background/40 p-4">
								<p class="text-xs tracking-[0.14em] text-muted-foreground uppercase">Receiver response</p>
								<div class="mt-3 space-y-2 text-sm">
									<p><span class="text-muted-foreground">Response code:</span> {headerValue(selectedDeliveryDetails.delivery.response_status_code)}</p>
									<p><span class="text-muted-foreground">Outcome:</span> {selectedDeliveryDetails.delivery.error ? 'Failed' : 'Accepted or pending'}</p>
								</div>
								{#if selectedDeliveryDetails.delivery.error}
									<pre class="mt-3 max-h-52 overflow-auto rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-xs leading-5 text-red-100">{selectedDeliveryDetails.delivery.error}</pre>
								{:else if selectedDeliveryDetails.delivery.response_body}
									<pre class="mt-3 max-h-52 overflow-auto rounded-lg border border-border/60 bg-muted/30 p-3 text-xs leading-5 text-muted-foreground">{selectedDeliveryDetails.delivery.response_body}</pre>
								{:else}
									<p class="mt-3 text-sm text-muted-foreground">No response body recorded.</p>
								{/if}
							</div>

							<div class="rounded-xl border border-amber-500/20 bg-amber-500/5 p-4">
								<p class="text-xs tracking-[0.14em] text-amber-300 uppercase">Redelivery</p>
								<p class="mt-2 text-sm text-muted-foreground">
									A per-delivery redeliver action is not wired up yet. For now, use the watcher-level
									replay controls or resend the test event after fixing the receiver.
								</p>
							</div>
						</div>
					</div>
				{:else}
					<div class="rounded-xl border border-border/60 bg-muted/20 p-4 text-sm text-muted-foreground">
						Select a delivery to inspect its payload and response history.
					</div>
				{/if}
			</div>

			<Dialog.Footer>
				<Button.Root type="button" variant="outline" onclick={closeDeliveryDetails}>Close</Button.Root>
			</Dialog.Footer>
		</Dialog.Content>
	</Dialog.Root>
</div>
