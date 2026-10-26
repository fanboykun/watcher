<script lang="ts">
	import AttemptTimeline from '$lib/features/deployments/attempt-timeline.svelte';
	import type { DeployLog, Watcher } from '$lib/api';
	let {
		deploys = [],
		deployPage = $bindable(1),
		deployPageSize = $bindable(10),
		deployTotal = 0,
		paging = false,
		watcher
	}: {
		deploys: DeployLog[];
		deployPage: number;
		deployPageSize: number;
		deployTotal: number;
		paging?: boolean;
		watcher: Watcher | null;
	} = $props();
</script>

{#if watcher}<AttemptTimeline
		attempts={deploys}
		watcherId={watcher.id}
		total={deployTotal}
		page={deployPage}
		pageSize={deployPageSize}
		{paging}
		onpage={(page) => {
			deployPage = page;
		}}
		onsize={(size) => {
			deployPageSize = size;
			deployPage = 1;
		}}
	/>{/if}
