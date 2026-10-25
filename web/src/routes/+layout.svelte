<script lang="ts">
	import './layout.css';
	import { QueryClientProvider } from '@tanstack/svelte-query';
	import { createQueryClient } from '$lib/query/client';
	import AuthGate from '$lib/components/shell/auth-gate.svelte';
	let { children } = $props();
	const client = createQueryClient();
</script>

<QueryClientProvider {client}><AuthGate>{@render children()}</AuthGate></QueryClientProvider>
