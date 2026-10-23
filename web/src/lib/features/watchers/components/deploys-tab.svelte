<script lang="ts">
	import AttemptTimeline from '$lib/features/deployments/attempt-timeline.svelte';
	import type { DeployLog } from '$lib/api';

	let {
		deploys,
		deployPage = $bindable(1),
		deployPageSize = $bindable(10),
		deployTotal,
		onPageChange,
		onPageSizeChange,
		watcherId
	}: {
		deploys: DeployLog[];
		deployPage: number;
		deployPageSize: number;
		deployTotal: number;
		onPageChange: (page: number) => void;
		onPageSizeChange: (size: number) => void;
		watcherId: number;
	} = $props();
</script>

<AttemptTimeline
	attempts={deploys}
	{watcherId}
	total={deployTotal}
	page={deployPage}
	pageSize={deployPageSize}
	onpage={onPageChange}
	onsize={onPageSizeChange}
/>
