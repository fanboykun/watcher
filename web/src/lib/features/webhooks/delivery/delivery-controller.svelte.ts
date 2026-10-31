import { createQuery } from '@tanstack/svelte-query';
import { queries } from '$lib/query/options';
import { message } from '$lib/query/client';
import { createCommand } from '$lib/query/command.svelte';
import { type Watcher, type WebhookDelivery } from '$lib/api';
export function createDelivery(
	getProps: () => {
		watcher: Watcher;
		busy?: boolean;
		deliveries: WebhookDelivery[];
		deliveryPage: number;
		deliveryPageSize: number;
		deliveryTotal: number;
		onPageChange: (page: number) => void | Promise<void>;
		onPageSizeChange: (size: number) => void | Promise<void>;
		onSendTest: () => void | Promise<void>;
		onResume: () => void | Promise<void>;
		onResumeReplay: () => void | Promise<void>;
	}
) {
	const {
		watcher,
		busy = false,
		deliveries,
		deliveryPage = 1,
		deliveryPageSize = 20,
		deliveryTotal,
		onPageChange,
		onPageSizeChange,
		onSendTest,
		onResume,
		onResumeReplay
	}: {
		watcher: Watcher;
		busy?: boolean;
		deliveries: WebhookDelivery[];
		deliveryPage: number;
		deliveryPageSize: number;
		deliveryTotal: number;
		onPageChange: (page: number) => void | Promise<void>;
		onPageSizeChange: (size: number) => void | Promise<void>;
		onSendTest: () => void | Promise<void>;
		onResume: () => void | Promise<void>;
		onResumeReplay: () => void | Promise<void>;
	} = $derived(getProps());

	const ui = $state({
		showDeliveryDetails: false,
		showReplay: false,
		replayError: '',
		selectedDeliveryID: null as number | null
	});

	const replayMutation = createCommand();
	const replaying = $derived(replayMutation.isPending);

	const deliveryQuery = createQuery(() => ({
		...queries.delivery(watcher.id, ui.selectedDeliveryID ?? 0),
		enabled: ui.showDeliveryDetails && ui.selectedDeliveryID !== null
	}));
	const selectedDeliveryDetails = $derived(deliveryQuery.data ?? null);
	const selectedDeliveryLoading = $derived(deliveryQuery.isPending);
	const selectedDeliveryError = $derived(message(deliveryQuery.error));

	const safeDeliveries = $derived(Array.isArray(deliveries) ? deliveries : []);

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
			return 'Paste the exact same signing secret into your receiver and Watcher. Raw base64 secret material or the conventional `whsec_...` form both work, and the masked text shown here is only a display hint.';
		}
		return 'Watcher will inherit the global signing secret from system settings. Raw base64 secret material or the conventional `whsec_...` form both work, and your receiver must use the same exact value to verify signatures.';
	}

	const deliverySummary = $derived.by<DeliverySummary>(() => {
		let successCount = 0;
		let failedCount = 0;
		let pendingCount = 0;
		let suppressedCount = 0;
		let lastSuccessful: WebhookDelivery | null = null;
		let lastFailed: WebhookDelivery | null = null;
		let nextRetry: WebhookDelivery | null = null;

		for (const delivery of safeDeliveries) {
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

	function openDeliveryDetails(delivery: WebhookDelivery) {
		ui.selectedDeliveryID = delivery.id;
		ui.showDeliveryDetails = true;
	}
	function closeDeliveryDetails() {
		ui.showDeliveryDetails = false;
		ui.selectedDeliveryID = null;
	}
	return {
		ui,
		get watcher() {
			return watcher;
		},
		get busy() {
			return busy;
		},
		get onSendTest() {
			return onSendTest;
		},
		get deliverySummary() {
			return deliverySummary;
		},
		get onResume() {
			return onResume;
		},
		routeSource,
		authSource,
		get safeDeliveries() {
			return safeDeliveries;
		},
		get deliveryPage() {
			return deliveryPage;
		},
		get deliveryPageSize() {
			return deliveryPageSize;
		},
		get deliveryTotal() {
			return deliveryTotal;
		},
		get onPageSizeChange() {
			return onPageSizeChange;
		},
		get onPageChange() {
			return onPageChange;
		},
		statusTone,
		statusLabel,
		preview,
		isSuppressed,
		isSucceeded,
		openDeliveryDetails,
		get selectedDeliveryLoading() {
			return selectedDeliveryLoading;
		},
		get selectedDeliveryError() {
			return selectedDeliveryError;
		},
		get selectedDeliveryDetails() {
			return selectedDeliveryDetails;
		},
		headerValue,
		prettyJson,
		closeDeliveryDetails,
		get replaying() {
			return replaying;
		},
		get replayMutation() {
			return replayMutation;
		},
		get onResumeReplay() {
			return onResumeReplay;
		}
	};
}
