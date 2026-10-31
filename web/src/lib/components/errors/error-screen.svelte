<script lang="ts">
	import type { Snippet } from 'svelte';
	import { resolve } from '$app/paths';
	import * as Button from '$lib/components/ui/button';
	import {
		ArrowLeft,
		Bug,
		Check,
		Copy,
		GitCompare,
		House,
		RotateCw,
		ScrollText,
		SearchX,
		ServerCrash,
		ShieldAlert,
		TimerOff,
		TriangleAlert,
		WifiOff
	} from '@lucide/svelte';
	import { describeError, type ErrorKind } from './error-model';

	let {
		error,
		status,
		title,
		scope = 'page',
		onRetry,
		actions
	}: {
		error: unknown;
		/** Router status; API errors carry their own. */
		status?: number;
		title?: string;
		/** `app` fills the viewport for failures outside the workspace shell. */
		scope?: 'page' | 'app';
		onRetry?: () => unknown;
		actions?: Snippet;
	} = $props();

	const uid = $props.id();
	const info = $derived(describeError(error, status));
	const icons: Record<ErrorKind, typeof SearchX> = {
		'not-found': SearchX,
		unauthorized: ShieldAlert,
		forbidden: ShieldAlert,
		offline: WifiOff,
		timeout: TimerOff,
		conflict: GitCompare,
		invalid: TriangleAlert,
		server: ServerCrash,
		unexpected: Bug
	};
	const Icon = $derived(icons[info.kind]);
	const showLogs = $derived(
		scope === 'page' && (info.kind === 'server' || info.kind === 'timeout')
	);
	let retrying = $state(false);
	let copied = $state(false);
	let detailsOpen = $state(false);

	async function retry() {
		if (!onRetry || retrying) return;
		retrying = true;
		try {
			await onRetry();
		} finally {
			retrying = false;
		}
	}

	async function copyDetails() {
		try {
			await navigator.clipboard.writeText(info.detail);
			copied = true;
			setTimeout(() => (copied = false), 2000);
		} catch {
			copied = false;
		}
	}

	function back() {
		if (history.length > 1) history.back();
		else location.assign(resolve('/'));
	}
</script>

<section
	class={['error-screen', scope === 'app' && 'error-screen-app']}
	data-kind={info.kind}
	role="alert"
	aria-labelledby={`${uid}-title`}
>
	<div class="error-panel">
		<span class="error-code" aria-hidden="true">{info.code}</span>
		<span class="error-glyph"><Icon class="size-6" /></span>
		<p class="eyebrow">Error <span class="opacity-40">/</span> {info.code}</p>
		<h1 id={`${uid}-title`} class="error-title">{title ?? info.title}</h1>
		{#if info.message}<p class="error-message">{info.message}</p>{/if}
		<p class="error-hint">{info.hint}</p>

		<div class="error-actions">
			{#if onRetry}
				<Button.Root onclick={retry} loading={retrying}
					>{#if !retrying}<RotateCw class="size-4" />{/if}Try again</Button.Root
				>
			{/if}
			{#if scope === 'app'}
				<Button.Root variant="outline" onclick={() => location.reload()}
					><RotateCw class="size-4" />Reload dashboard</Button.Root
				>
			{:else}
				<Button.Root variant="outline" onclick={back}
					><ArrowLeft class="size-4" />Go back</Button.Root
				>
				<Button.Root variant="ghost" href={resolve('/')}
					><House class="size-4" />Dashboard</Button.Root
				>
			{/if}
			{#if showLogs}
				<Button.Root variant="ghost" href={resolve('/logs')}
					><ScrollText class="size-4" />Agent log</Button.Root
				>
			{/if}
			{@render actions?.()}
		</div>

		<details class="error-details" bind:open={detailsOpen}>
			<summary
				>Technical details{#if info.requestId}<span class="normal-case"
						>· request {info.requestId}</span
					>{/if}</summary
			>
			{#if detailsOpen}
				<pre>{info.detail}</pre>
				<Button.Root variant="outline" size="sm" onclick={copyDetails}>
					{#if copied}<Check class="size-3.5" />Copied{:else}<Copy class="size-3.5" />Copy details{/if}
				</Button.Root>
			{/if}
		</details>
	</div>
</section>
