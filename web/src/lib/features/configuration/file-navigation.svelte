<script lang="ts">
	import type { ServiceConfigFile } from '$lib/api';
	import * as Button from '$lib/components/ui/button';
	import { FileCode, FilePlus, FileX } from '@lucide/svelte';
	import { fileLocation } from './model';
	let {
		serviceName,
		files,
		envPath,
		envContent,
		baselineEnv,
		baselineFiles,
		selectedFile,
		removedSelection,
		removed,
		changedCount,
		mode,
		explicitFiles,
		disabled,
		onadd,
		onchoose,
		onremoved,
		oncopy
	}: {
		serviceName: string;
		files: ServiceConfigFile[];
		envPath: string;
		envContent: string;
		baselineEnv: string;
		baselineFiles: ServiceConfigFile[];
		selectedFile?: ServiceConfigFile;
		removedSelection: ServiceConfigFile | null;
		removed: ServiceConfigFile[];
		changedCount: number;
		mode: string;
		explicitFiles: boolean;
		disabled: boolean;
		onadd: () => void;
		onchoose: (index: number) => void;
		onremoved: (file: ServiceConfigFile) => void;
		oncopy: () => void;
	} = $props();
</script>

<aside class="config-files" aria-label={`${serviceName} files`}>
	<div class="flex items-center justify-between px-3 pb-3">
		<span class="eyebrow">Files · {files.length + 1}</span><Button.Root
			variant="ghost"
			size="icon-sm"
			aria-label="Add config file"
			onclick={onadd}
			{disabled}><FilePlus class="size-4" /></Button.Root
		>
	</div>
	<button
		class={['config-file', !selectedFile && !removedSelection && 'config-file-selected']}
		onclick={() => onchoose(-1)}
		aria-label={envPath || 'Environment'}
		title={envContent !== baselineEnv ? 'Changed from active configuration' : undefined}
		aria-pressed={!selectedFile && !removedSelection}
		><FileCode class="size-4 shrink-0" /><span class="min-w-0 flex-1 truncate"
			>{envPath || 'Environment'}</span
		>{#if envContent !== baselineEnv}<span class="config-change-dot" aria-label="Changed"
			></span>{/if}</button
	>
	{#each files as file, index (file)}
		{@const baseline = baselineFiles.find((entry) => fileLocation(entry) === fileLocation(file))}
		<button
			class={['config-file', selectedFile === file && !removedSelection && 'config-file-selected']}
			onclick={() => onchoose(index)}
			aria-label={file.file_path || 'Untitled file'}
			title={!baseline
				? 'Added file'
				: baseline.content !== file.content
					? 'Changed from active configuration'
					: undefined}
			aria-pressed={selectedFile === file && !removedSelection}
			><FileCode class="size-4 shrink-0" /><span class="min-w-0 flex-1 truncate"
				>{file.file_path || 'Untitled file'}</span
			>{#if !baseline || baseline.content !== file.content}<span
					class="config-change-dot"
					aria-label={baseline ? 'Changed' : 'Added'}
				></span>{/if}</button
		>
	{/each}
	{#if removed.length}<p class="eyebrow mt-5 px-3 pb-2">Removed from set</p>
		{#each removed as file (fileLocation(file))}<button
				class={['config-file text-red-400', removedSelection === file && 'config-file-selected']}
				onclick={() => {
					onremoved(file);
				}}
				aria-label={file.file_path}
				title="Removed from the managed file set"
				><FileX class="size-4 shrink-0" /><span class="truncate">{file.file_path}</span></button
			>{/each}{/if}
	<div
		class="mt-4 border-t border-border px-3 pt-4 text-[11px] leading-relaxed text-muted-foreground"
	>
		{changedCount} file{changedCount === 1 ? '' : 's'} differ from active configuration.
		{#if mode === 'candidate'}<p class="mt-3">
				{explicitFiles
					? 'An explicit file set is staged.'
					: 'Managed files inherit active settings until edited.'}
			</p>
			<Button.Root
				variant="ghost"
				size="xs"
				class="mt-2 h-auto text-left whitespace-normal"
				{disabled}
				onclick={oncopy}>{explicitFiles ? 'Copy active files' : 'Stage active files'}</Button.Root
			>{/if}
	</div>
</aside>
