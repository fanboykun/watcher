<script lang="ts">
	import AttemptTimeline from '$lib/features/deployments/attempt-timeline.svelte';
	import type { DeployLog } from '$lib/api';

	let {
		deploys,
		deployPage = $bindable(1),
		deployPageSize = $bindable(10),
		deployTotal,
		paging = false,
		onPageChange,
		onPageSizeChange,
		watcherId
	}: {
		deploys: DeployLog[];
		deployPage: number;
		deployPageSize: number;
		deployTotal: number;
		paging?: boolean;
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
	{paging}
	onpage={onPageChange}
	onsize={onPageSizeChange}
/>
