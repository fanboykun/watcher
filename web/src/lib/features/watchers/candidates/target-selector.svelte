<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Layers, Pencil, RefreshCw, Save, Zap } from '@lucide/svelte';
	import type { createCandidate } from '$lib/features/watchers/candidates/candidate-controller.svelte';
	let { model }: { model: ReturnType<typeof createCandidate> } = $props();
</script>

<Card.Root class="border-border bg-card">
	<Card.Header class="flex flex-col items-stretch pb-3">
		<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
			<div>
				<p class="eyebrow mb-2">01 / Release target</p>
				<Card.Title class="text-base font-semibold">Candidate configuration</Card.Title>
				<Card.Description class="text-xs">
					Select the pending release, the next release, or a saved version draft. Changes remain
					isolated from active settings.
				</Card.Description>
			</div>

			<div class="flex flex-wrap items-center gap-2">
				{#if model.hasAnyChanges}
					<span class="text-xs font-medium text-amber-400">Unsaved edits</span>
				{/if}
				<Button.Root
					variant="default"
					size="sm"
					class="h-8"
					onclick={model.saveAllCandidates}
					disabled={model.busy ||
						model.saving ||
						model.approving ||
						model.discarding ||
						model.loading ||
						!model.effectiveTarget ||
						model.ui.loadedTarget !== model.effectiveTarget}
				>
					{#if model.saving}
						<RefreshCw class="mr-1.5 h-3.5 w-3.5 animate-spin" /> Saving...
					{:else}
						<Save class="mr-1.5 h-3.5 w-3.5" /> Save candidate
					{/if}
				</Button.Root>
			</div>
		</div>

		<!-- Quick Target Selector Pills -->
		<div class="mt-3 flex flex-wrap items-center gap-2">
			{#if model.watcher.pending_version}
				<Button.Root
					variant="ghost"
					size="sm"
					type="button"
					class={`inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-semibold transition ${
						!model.ui.isCustom && model.activeTargetChoice === model.watcher.pending_version
							? 'bg-primary text-white shadow-sm'
							: 'border border-primary/40 bg-primary/10 text-primary hover:bg-primary/20'
					}`}
					onclick={() => model.selectTarget(model.watcher.pending_version)}
				>
					<Zap class="h-3 w-3" />
					{model.watcher.pending_version}
					<span class="py-0.2 rounded bg-black/20 px-1 text-[10px] tracking-wider uppercase">
						Candidate
					</span>
				</Button.Root>
			{/if}

			<Button.Root
				variant="ghost"
				size="sm"
				type="button"
				class={`inline-flex items-center gap-1 rounded-full px-3 py-1 text-xs font-medium transition ${
					!model.ui.isCustom && model.activeTargetChoice === 'next'
						? 'bg-primary text-primary-foreground shadow-sm hover:bg-primary/90'
						: 'border border-border bg-muted/40 text-muted-foreground hover:bg-muted'
				}`}
				onclick={() => model.selectTarget('next')}
			>
				<Layers class="h-3 w-3" />
				Next release
			</Button.Root>

			<Button.Root
				variant="ghost"
				size="sm"
				type="button"
				class={`inline-flex items-center gap-1 rounded-full px-3 py-1 text-xs font-medium transition ${
					model.ui.isCustom
						? 'bg-primary text-primary-foreground shadow-sm hover:bg-primary/90'
						: 'border border-border bg-muted/40 text-muted-foreground hover:bg-muted'
				}`}
				onclick={() => model.selectTarget(model.ui.customTarget || 'custom', true)}
			>
				<Pencil class="h-3 w-3" />
				Specific version
			</Button.Root>

			{#if model.ui.isCustom}
				<div class="flex items-center gap-1.5 pl-1">
					<Input
						bind:value={model.ui.customTarget}
						aria-label="Candidate target version"
						placeholder="e.g. v2.0.0"
						class="h-7 w-36 font-mono text-xs"
						onkeydown={(e) => {
							if (e.key === 'Enter') model.handleCustomTargetSubmit();
						}}
					/>
					<Button.Root
						variant="outline"
						size="sm"
						class="h-7 px-2 text-xs"
						onclick={model.handleCustomTargetSubmit}
					>
						Load
					</Button.Root>
				</div>
			{/if}
		</div>
		{#if model.ui.isCustom && model.ui.customTarget.trim() !== model.effectiveTarget}<p
				class="mt-3 text-xs text-muted-foreground"
			>
				Load this version to switch drafts.{#if model.ui.loadedTarget}
					Current draft: <span class="font-mono">{model.ui.loadedTarget}</span>.{/if}
			</p>{/if}
		{#if model.ui.savedTargets.some((target) => target !== 'next' && target !== model.watcher.pending_version)}
			<div class="mt-3 flex flex-wrap items-center gap-2" aria-label="Saved candidate targets">
				<span class="text-xs text-muted-foreground">Saved drafts:</span>
				{#each model.ui.savedTargets.filter((target) => target !== 'next' && target !== model.watcher.pending_version) as target (target)}
					<Button.Root
						variant="outline"
						size="sm"
						type="button"
						onclick={() => model.selectTarget(target)}>{target}</Button.Root
					>
				{/each}
			</div>
		{/if}
	</Card.Header>
</Card.Root>
