<script lang="ts">
	import type { ServiceConfigFile } from '$lib/api';
	import * as Button from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Plus, Trash2 } from '@lucide/svelte';

	let {
		serviceName,
		activeFiles,
		files = $bindable([]),
		explicit,
		onChange
	}: {
		serviceName: string;
		activeFiles: ServiceConfigFile[];
		files: ServiceConfigFile[];
		explicit: boolean;
		onChange: () => void;
	} = $props();

	function sameLocation(a: ServiceConfigFile, b: ServiceConfigFile) {
		return a.target === b.target && a.file_path === b.file_path;
	}

	const removedFiles = $derived(
		activeFiles.filter((active) => !files.some((file) => sameLocation(active, file)))
	);

	function addFile() {
		files = [...files, { file_path: '', target: 'app_dir', content: '' }];
		onChange();
	}

	function removeFile(file: ServiceConfigFile) {
		files = files.filter((entry) => entry !== file);
		onChange();
	}

	function stageActive() {
		files = activeFiles.map(({ file_path, target, content }) => ({ file_path, target, content }));
		onChange();
	}
</script>

<section
	class="min-w-0 space-y-3 border-t border-border pt-4"
	aria-label={`${serviceName} managed config files`}
>
	<div class="section-toolbar">
		<div class="min-w-0 space-y-1">
			<h3 class="text-sm font-medium">Managed config files</h3>
			<p class="text-xs text-muted-foreground">
				{#if explicit}
					This candidate replaces the managed file set when deployed.
				{:else}
					Inherits active files at deployment. Edit a file to stage a separate set.
				{/if}
				Saving only prepares the candidate; active settings and live files stay unchanged.
			</p>
		</div>
		<div class="flex shrink-0 flex-wrap items-center gap-2">
			<Button.Root type="button" variant="ghost" size="sm" onclick={stageActive}>
				{explicit ? 'Copy active files' : 'Stage active files'}
			</Button.Root>
			<Button.Root type="button" variant="outline" size="sm" onclick={addFile}>
				<Plus class="mr-1 h-3.5 w-3.5" /> Add config file
			</Button.Root>
		</div>
	</div>

	{#each files as file, index (file)}
		{@const active = activeFiles.find((entry) => sameLocation(entry, file))}
		<div class="min-w-0 space-y-3 rounded-lg border border-border bg-background/50 p-3 sm:p-4">
			<div class="flex items-center justify-between gap-2">
				<p class="text-xs font-medium">
					Candidate file {index + 1}
					<span class="ml-1 text-muted-foreground"
						>{!active
							? 'Added'
							: active.content !== file.content
								? 'Changed'
								: 'Matches active'}</span
					>
				</p>
				<Button.Root
					type="button"
					variant="ghost"
					size="sm"
					onclick={() => removeFile(file)}
					aria-label={`Remove config file ${index + 1}`}
				>
					<Trash2 class="mr-1 h-3.5 w-3.5" /> Remove
				</Button.Root>
			</div>
			<div class="grid min-w-0 gap-3 sm:grid-cols-[minmax(0,1fr)_180px]">
				<label class="min-w-0 space-y-1.5 text-xs font-medium">
					<span>Relative path</span>
					<Input
						bind:value={file.file_path}
						oninput={onChange}
						aria-label={`Config file ${index + 1} path`}
						placeholder="settings/appsettings.json"
						class="font-mono text-xs"
					/>
				</label>
				<label class="space-y-1.5 text-xs font-medium">
					<span>Target directory</span>
					<select
						bind:value={file.target}
						onchange={onChange}
						aria-label={`Config file ${index + 1} target`}
						class="flex h-9 w-full min-w-0 rounded-md border border-input bg-background px-3 text-sm shadow-xs focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
					>
						<option value="app_dir">Service/app dir</option>
						<option value="release_dir">Release dir</option>
					</select>
				</label>
			</div>
			<label class="block space-y-1.5 text-xs font-medium">
				<span>Candidate contents</span>
				<Textarea
					bind:value={file.content}
					oninput={onChange}
					aria-label={`Config file ${index + 1} contents`}
					class="min-h-[140px] font-mono text-xs"
					placeholder="File contents"
				/>
			</label>
			<details class="min-w-0 text-xs text-muted-foreground">
				<summary class="cursor-pointer">Review active file</summary>
				{#if active}
					<p class="mt-2 font-mono break-all">{active.target}/{active.file_path}</p>
					<pre
						class="mt-2 max-h-56 overflow-auto rounded bg-muted/40 p-3 break-all whitespace-pre-wrap">{active.content ||
							'(empty file)'}</pre>
				{:else}
					<p class="mt-2">No active managed file at this path and target.</p>
				{/if}
			</details>
		</div>
	{:else}
		<p class="rounded-md border border-dashed border-border p-4 text-xs text-muted-foreground">
			{explicit
				? 'No managed config files will be included in this candidate.'
				: 'No active managed config files.'}
		</p>
	{/each}

	{#if removedFiles.length > 0}
		<div class="space-y-2 rounded-md border border-amber-500/30 bg-amber-500/5 p-3 text-xs">
			<p class="font-medium text-amber-400">Removed from the candidate managed file set</p>
			{#each removedFiles as file (file)}
				<details class="min-w-0 text-muted-foreground">
					<summary class="cursor-pointer font-mono break-all"
						>{file.target}/{file.file_path}</summary
					>
					<pre
						class="mt-2 max-h-56 overflow-auto rounded bg-muted/40 p-3 break-all whitespace-pre-wrap">{file.content ||
							'(empty file)'}</pre>
				</details>
			{/each}
		</div>
	{/if}
	<p class="text-xs text-muted-foreground">
		Use a relative file path inside the service/app or release directory. Release-dir files sit
		beside the deployed assets.
	</p>
</section>
