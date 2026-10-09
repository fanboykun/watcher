<script lang="ts">
	import LogBrowser from '$lib/components/log-browser.svelte';
</script>

<div class="space-y-6">
	<header>
		<h1 class="text-2xl font-semibold tracking-tight">Logs</h1>
		<p class="mt-1 text-sm text-muted-foreground">
			Agent activity and Watcher's Windows service output
		</p>
	</header>
	<LogBrowser />
</div>
