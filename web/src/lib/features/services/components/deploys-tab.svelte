<script lang="ts">
	import AttemptTimeline from '$lib/features/deployments/attempt-timeline.svelte';
	import type { DeployLog, Watcher } from '$lib/api';
	let {
		deploys = $bindable([]),
		deployPage = $bindable(1),
		deployPageSize = $bindable(10),
		deployTotal = 0,
		watcher,
		onLoadDeploys
	}: {
		deploys: DeployLog[];
		deployPage: number;
		deployPageSize: number;
		deployTotal: number;
		watcher: Watcher | null;
		onLoadDeploys: () => Promise<void>;
	} = $props();
</script>

{#if watcher}<AttemptTimeline
		attempts={deploys}
		watcherId={watcher.id}
		total={deployTotal}
		page={deployPage}
		pageSize={deployPageSize}
		onpage={async (page) => {
			deployPage = page;
			await onLoadDeploys();
		}}
		onsize={async (size) => {
			deployPageSize = size;
			deployPage = 1;
			await onLoadDeploys();
		}}
	/>{/if}
