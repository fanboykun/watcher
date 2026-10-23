<script lang="ts">
	import type { ComponentProps } from 'svelte';
	import ServiceForm from '$lib/features/services/service-form.svelte';
	let { initial = {}, ...props }: ComponentProps<typeof ServiceForm> = $props();
</script>

{#key initial}
	<ServiceForm {initial} {...props} />
{/key}
