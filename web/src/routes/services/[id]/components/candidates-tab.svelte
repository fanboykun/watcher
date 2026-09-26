<script lang="ts">
	import type { ServiceConfigRevision } from '$lib/api';
	import * as api from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Trash2, Save, RefreshCw, Plus } from '@lucide/svelte';
	import { onMount } from 'svelte';

	let { serviceId, currentEnv }: { serviceId: number, currentEnv: string } = $props();

	let revisions = $state<ServiceConfigRevision[]>([]);
	let loading = $state(true);
	let error = $state('');
	
	let newTarget = $state('next');
	let newContent = $state(currentEnv);
	let saving = $state(false);

	async function loadRevisions() {
		try {
			const res = await api.getServiceConfigRevisions(serviceId);
			revisions = res.data || [];
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load config candidates';
		} finally {
			loading = false;
		}
	}

	async function saveRevision(target: string, content: string) {
		if (!target.trim()) return;
		saving = true;
		try {
			await api.updateServiceConfigRevision(serviceId, target.trim(), content);
			await loadRevisions();
			if (target === newTarget) {
				newTarget = 'next';
				newContent = currentEnv;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save config candidate';
		} finally {
			saving = false;
		}
	}

	async function deleteRevision(target: string) {
		try {
			await api.deleteServiceConfigRevision(serviceId, target);
			await loadRevisions();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to delete config candidate';
		}
	}

	onMount(() => {
		loadRevisions();
	});
</script>

<div class="space-y-6">
	<Card.Root class="border-border bg-card">
		<Card.Header class="pb-3">
			<Card.Title class="text-lg">Pre-configure Candidate</Card.Title>
			<Card.Description>Prepare configuration for an upcoming version (e.g. <code>v2.0.0-rc1</code>) or the <code>next</code> deployment.</Card.Description>
		</Card.Header>
		<Card.Content class="space-y-4">
			<div class="grid gap-2">
				<p class="text-sm text-muted-foreground">Target Version</p>
				<Input bind:value={newTarget} placeholder="e.g. next, v2.0.0" />
			</div>
			<div class="grid gap-2">
				
			<div class="flex items-center justify-between">
				<p class="text-sm text-muted-foreground">Environment Variables</p>
				<Button.Root variant="ghost" size="sm" class="h-6 text-xs" onclick={() => newContent = currentEnv}>
					<RefreshCw class="mr-1 h-3 w-3" /> Reset to Active
				</Button.Root>
			</div>
			<Textarea
					bind:value={newContent}
					class="min-h-[150px] font-mono text-sm text-blue-300"
					placeholder="KEY=VALUE"
				/>
			</div>
			<Button.Root variant="default" onclick={() => saveRevision(newTarget, newContent)} disabled={saving || !newTarget.trim()}>
				<Plus class="mr-2 h-4 w-4" /> Add Candidate Config
			</Button.Root>
		</Card.Content>
	</Card.Root>

	{#if error}
		<div class="rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-400">
			{error}
		</div>
	{/if}

	{#if loading}
		<div class="p-4 text-sm text-muted-foreground">Loading candidates...</div>
	{:else}
		<div class="space-y-4">
			{#each revisions as rev}
				<Card.Root class="border-border bg-card">
					<Card.Header class="pb-2">
						<div class="flex items-center justify-between">
							<Card.Title class="text-md font-mono">{rev.target_version}</Card.Title>
							<div class="flex items-center gap-2">
								<Button.Root variant="outline" size="sm" onclick={() => saveRevision(rev.target_version, rev.env_content)}>
									<Save class="mr-2 h-4 w-4" /> Save
								</Button.Root>
								<Button.Root variant="ghost" size="icon" class="text-red-400" onclick={() => deleteRevision(rev.target_version)}>
									<Trash2 class="h-4 w-4" />
								</Button.Root>
							</div>
						</div>
						<Card.Description class="text-xs">Last updated: {new Date(rev.updated_at).toLocaleString()}</Card.Description>
					</Card.Header>
					<Card.Content>
						<Textarea
							bind:value={rev.env_content}
							class="min-h-[150px] font-mono text-sm text-blue-300"
						/>
					</Card.Content>
				</Card.Root>
			{/each}
			{#if revisions.length === 0}
				<div class="rounded-md border border-dashed border-border bg-muted/20 p-8 text-center text-sm text-muted-foreground">
					No active deployment candidates.
				</div>
			{/if}
		</div>
	{/if}
</div>
