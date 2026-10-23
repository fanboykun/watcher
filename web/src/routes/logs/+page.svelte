<script lang="ts">
	import PageHeading from '$lib/components/console/page-heading.svelte';
	import LogBrowser from '$lib/components/log-browser.svelte';
</script>

<div class="space-y-6">
	<PageHeading
		eyebrow="Observability / Activity"
		title="Agent logs"
		description="Follow the agent's activity, trace a deployment, and find the signal in the noise."
	/>
	<LogBrowser />
</div>
