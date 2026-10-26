<script lang="ts">
	import './layout.css';
	import { QueryClientProvider } from '@tanstack/svelte-query';
	import { page } from '$app/state';
	import { createQueryClient } from '$lib/query/client';
	import AuthGate from '$lib/components/shell/auth-gate.svelte';
	import ErrorScreen from '$lib/components/errors/error-screen.svelte';
	let { children } = $props();
	const client = createQueryClient();

	function report(error: unknown) {
		console.error('[watcher] render failure', error);
	}
</script>

<QueryClientProvider {client}>
	<svelte:boundary onerror={report}>
		<AuthGate>
			<!-- One page instance per resource: parameter changes remount the page, and a crash stays
			     inside the workspace shell until the operator navigates elsewhere. -->
			{#key page.url.pathname}
				<svelte:boundary onerror={report}>
					{@render children()}
					{#snippet failed(error, reset)}
						<ErrorScreen {error} onRetry={reset} />
					{/snippet}
				</svelte:boundary>
			{/key}
		</AuthGate>
		{#snippet failed(error, reset)}
			<ErrorScreen {error} scope="app" onRetry={reset} />
		{/snippet}
	</svelte:boundary>
</QueryClientProvider>
