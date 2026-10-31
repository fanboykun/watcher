<script lang="ts">
	import * as RadioGroup from '$lib/components/ui/radio-group';
	import { Label } from '$lib/components/ui/label';
	import type { ReleaseSourceType } from '$lib/api';

	let {
		value = $bindable(),
		idPrefix,
		disabled = false
	}: { value: ReleaseSourceType; idPrefix: string; disabled?: boolean } = $props();

	const options: { value: ReleaseSourceType; title: string; description: string }[] = [
		{
			value: 'github',
			title: 'GitHub',
			description: 'Repository releases or a version.json release asset.'
		},
		{
			value: 'http',
			title: 'Internal release feed',
			description: 'A version.json feed on company HTTP(S) storage. Hosts never contact GitHub.'
		},
		{
			value: 'fs',
			title: 'File share feed',
			description: 'The same feed on a local directory or SMB/UNC share.'
		},
		{
			value: 's3',
			title: 'S3 / MinIO feed',
			description: 'The same feed in a private S3-compatible bucket, signed with an access key.'
		}
	];
</script>

<RadioGroup.Root
	{value}
	onValueChange={(next) => {
		if (next) value = next as ReleaseSourceType;
	}}
	aria-label="Release source"
	{disabled}
	class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4"
>
	{#each options as option (option.value)}
		<Label
			for="{idPrefix}-{option.value}"
			class="flex cursor-pointer items-start gap-3 rounded-lg border border-border/70 p-3"
		>
			<RadioGroup.Item id="{idPrefix}-{option.value}" value={option.value} class="mt-1" />
			<div>
				<p class="text-sm font-medium">{option.title}</p>
				<p class="text-xs text-muted-foreground">{option.description}</p>
			</div>
		</Label>
	{/each}
</RadioGroup.Root>
