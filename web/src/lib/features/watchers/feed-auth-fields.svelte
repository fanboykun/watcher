<script lang="ts">
	import * as Select from '$lib/components/ui/select';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import type { FeedAuthDraft, FeedAuthType } from '$lib/api';

	let {
		auth = $bindable(),
		idPrefix,
		s3 = false,
		savedSecretMasked = ''
	}: {
		auth: FeedAuthDraft;
		idPrefix: string;
		/** S3 feeds always sign with an access key (SigV4), so the kind is fixed. */
		s3?: boolean;
		/** Masked saved secret; when set, a blank secret keeps it. */
		savedSecretMasked?: string;
	} = $props();

	const kinds: { value: FeedAuthType; label: string }[] = [
		{ value: '', label: 'None' },
		{ value: 'bearer', label: 'Bearer token' },
		{ value: 'basic', label: 'Basic auth' },
		{ value: 'header', label: 'Custom header' }
	];
	const selected = $derived(kinds.find((kind) => kind.value === auth.type) ?? kinds[0]);
	const kind = $derived(s3 ? 'sigv4' : selected.value);
	const secretPlaceholder = $derived(
		savedSecretMasked ? `Saved: ${savedSecretMasked} — leave blank to keep` : ''
	);
</script>

<div class="space-y-3 rounded-lg border border-border/70 bg-muted/20 p-4">
	{#if s3}
		<div class="space-y-1">
			<p class="text-sm font-medium">Bucket credentials</p>
			<p class="text-xs text-muted-foreground">
				A read-only access key for the bucket. Requests to the bucket's host are signed with AWS
				Signature V4; other hosts never receive it.
			</p>
		</div>
	{:else}
		<div class="space-y-2">
			<Label for="{idPrefix}-type">Feed credentials</Label>
			<Select.Root
				type="single"
				value={auth.type || 'none'}
				onValueChange={(value) => {
					auth.type = value === 'none' ? '' : (value as FeedAuthType);
				}}
			>
				<Select.Trigger id="{idPrefix}-type" class="w-48">{selected.label}</Select.Trigger>
				<Select.Content>
					{#each kinds as option (option.value)}
						<Select.Item value={option.value || 'none'} label={option.label}
							>{option.label}</Select.Item
						>
					{/each}
				</Select.Content>
			</Select.Root>
			<p class="text-xs text-muted-foreground">
				Sent only to the feed's own host, never to other hosts an artifact URL or redirect points
				to.
			</p>
		</div>
	{/if}
	{#if kind}
		<div class="grid gap-3 sm:grid-cols-2">
			{#if kind === 'basic' || kind === 'sigv4'}
				<div class="space-y-2">
					<Label for="{idPrefix}-username">{kind === 'sigv4' ? 'Access key ID' : 'Username'}</Label>
					<Input id="{idPrefix}-username" bind:value={auth.username} autocomplete="off" />
				</div>
			{:else if kind === 'header'}
				<div class="space-y-2">
					<Label for="{idPrefix}-header">Header name</Label>
					<Input id="{idPrefix}-header" bind:value={auth.header} placeholder="X-JFrog-Art-Api" />
				</div>
			{/if}
			<div class="space-y-2">
				<Label for="{idPrefix}-secret"
					>{kind === 'sigv4'
						? 'Secret access key'
						: kind === 'basic'
							? 'Password'
							: kind === 'header'
								? 'Value'
								: 'Token'}</Label
				>
				<Input
					id="{idPrefix}-secret"
					type="password"
					bind:value={auth.secret}
					autocomplete="new-password"
					placeholder={secretPlaceholder}
				/>
			</div>
			{#if kind === 'sigv4'}
				<div class="space-y-2">
					<Label for="{idPrefix}-region">Region</Label>
					<Input id="{idPrefix}-region" bind:value={auth.region} placeholder="us-east-1" />
				</div>
			{/if}
		</div>
	{/if}
</div>
