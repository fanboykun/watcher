<script lang="ts">
	import { createMutation, useQueryClient } from '@tanstack/svelte-query';
	import { api, type ReleaseMark } from '$lib/api';
	import { invalidate, watcherScope } from '$lib/query/client';
	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Ban, ShieldCheck } from '@lucide/svelte';
	import { isBlocked } from './release-marks';

	let {
		watcherId,
		version = '',
		mark,
		disabled = false
	}: {
		watcherId: number;
		/** Empty asks for the version, to block a release before this host ever receives it. */
		version?: string;
		mark?: ReleaseMark;
		disabled?: boolean;
	} = $props();

	const client = useQueryClient();
	const blocked = $derived(isBlocked(mark));
	// Fixed when the dialog opens, so its text does not flip when the mark refreshes on success.
	let unblocking = $state(false);
	let open = $state(false);
	let reason = $state('');
	let typedVersion = $state('');
	const target = $derived(version || typedVersion.trim());
	let error = $state('');

	const change = createMutation(() => ({
		mutationFn: () =>
			unblocking
				? api.unblockRelease(watcherId, target)
				: api.blockRelease(watcherId, target, reason.trim()),
		// Marks appear in the versions list, the catalog and the candidate review.
		onSuccess: () => invalidate(client, ...watcherScope(watcherId))
	}));

	function show() {
		unblocking = blocked;
		reason = '';
		typedVersion = '';
		error = '';
		open = true;
	}

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (change.isPending || !target || (!unblocking && !reason.trim())) return;
		error = '';
		try {
			await change.mutateAsync();
			open = false;
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not update the release.';
		}
	}
</script>

<Button.Root
	variant="outline"
	size="sm"
	{disabled}
	aria-label={version ? `${blocked ? 'Unblock' : 'Block'} ${version}` : undefined}
	onclick={show}
	>{#if blocked}<ShieldCheck />Unblock{:else}<Ban class="text-red-400" />{version
			? 'Block'
			: 'Block a version…'}{/if}</Button.Root
>

<Dialog.Root
	bind:open
	onOpenChange={(value) => {
		if (change.isPending && !value) open = true;
	}}
>
	<Dialog.Content class="sm:max-w-120" showCloseButton={!change.isPending}>
		<form class="space-y-4" onsubmit={submit}>
			<Dialog.Header>
				<Dialog.Title>{unblocking ? 'Unblock' : 'Block'} {version || 'a version'}?</Dialog.Title>
				<Dialog.Description>
					{#if unblocking}
						Watcher may deploy this version again: polling, approval, redeploy, catalog deployment
						and rollback all accept it.
					{:else}
						Watcher will never deploy this version: polling skips it, approval and redeploy are
						refused, and rollback, including automatic rollback, will not restore it. A deployment
						already running is not stopped.
					{/if}
				</Dialog.Description>
			</Dialog.Header>
			{#if unblocking}
				{#if mark?.block_reason}<p class="text-sm break-words text-muted-foreground">
						Blocked because: {mark.block_reason}
					</p>{/if}
			{:else}
				{#if !version}<div class="space-y-2">
						<Label for="block-version">Version</Label>
						<Input
							id="block-version"
							bind:value={typedVersion}
							required
							placeholder="v1.4.2"
							class="font-mono"
							disabled={change.isPending}
						/>
						<p class="text-xs text-muted-foreground">
							Use the exact version Watcher reports, for example one another host found broken.
						</p>
					</div>{/if}
				<div class="space-y-2">
					<Label for="block-reason">Reason</Label>
					<Textarea
						id="block-reason"
						bind:value={reason}
						maxlength={500}
						required
						placeholder="What is wrong with this release?"
						disabled={change.isPending}
					/>
				</div>
			{/if}
			{#if error}<p role="alert" class="text-sm break-words text-red-400">{error}</p>{/if}
			<Dialog.Footer>
				<Button.Root
					variant="outline"
					type="button"
					disabled={change.isPending}
					onclick={() => (open = false)}>Cancel</Button.Root
				>
				<Button.Root
					type="submit"
					variant={unblocking ? 'default' : 'destructive'}
					loading={change.isPending}
					disabled={change.isPending || !target || (!unblocking && !reason.trim())}
					>{unblocking ? 'Unblock release' : 'Block release'}</Button.Root
				>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
