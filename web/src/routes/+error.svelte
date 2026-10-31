<script lang="ts">
	import { page } from '$app/state';
	import ErrorScreen from '$lib/components/errors/error-screen.svelte';
</script>

<svelte:head>
	<title>{page.status} | Watcher</title>
</svelte:head>

<ErrorScreen
	error={page.status === 404
		? { message: `No dashboard view at ${page.url.pathname}.` }
		: page.error}
	status={page.status}
	title={page.status === 404 ? 'This page does not exist' : undefined}
/>
