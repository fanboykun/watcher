<script lang="ts">
	import type { ServiceConfigFile } from '$lib/api';
	import type { Snippet } from 'svelte';
	import {
		FileCode,
		FilePlus,
		FileX,
		Pencil,
		Columns2,
		RotateCcw,
		Trash2,
		Check,
		Braces
	} from '@lucide/svelte';
	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Input } from '$lib/components/ui/input';
	import CodeEditor from './code-editor.svelte';
	import { configurationProblems, fileLocation, copyFiles } from './model';
	let {
		serviceName,
		context,
		mode = 'candidate',
		directory = '',
		envPath = '',
		envContent = $bindable(''),
		files = $bindable([]),
		baselineEnv = '',
		baselineFiles = [],
		savedEnv = '',
		savedFiles = [],
		explicitFiles = true,
		savedExplicitFiles = true,
		disabled = false,
		onfileschange,
		actions
	}: {
		serviceName: string;
		context: string;
		mode?: 'active' | 'candidate' | 'form';
		directory?: string;
		envPath?: string;
		envContent?: string;
		files?: ServiceConfigFile[];
		baselineEnv?: string;
		baselineFiles?: ServiceConfigFile[];
		savedEnv?: string;
		savedFiles?: ServiceConfigFile[];
		explicitFiles?: boolean;
		savedExplicitFiles?: boolean;
		disabled?: boolean;
		onfileschange?: () => void;
		actions?: Snippet;
	} = $props();
	let selected = $state(-1);
	let removedSelection = $state<ServiceConfigFile | null>(null);
	let review = $state(false);
	let confirmation = $state<'remove' | 'restore' | 'copy' | null>(null);
	const selectedFile = $derived(selected >= 0 ? files[selected] : undefined);
	const original = $derived(
		selectedFile
			? (baselineFiles.find((file) => fileLocation(file) === fileLocation(selectedFile))?.content ??
					'')
			: baselineEnv
	);
	const currentContent = $derived(removedSelection ? '' : (selectedFile?.content ?? envContent));
	const currentPath = $derived(
		removedSelection?.file_path || selectedFile?.file_path || envPath || '.env'
	);
	const removed = $derived(
		explicitFiles
			? baselineFiles.filter(
					(file) => !files.some((current) => fileLocation(current) === fileLocation(file))
				)
			: []
	);
	const changedCount = $derived(
		Number(envContent !== baselineEnv) +
			files.filter(
				(file) =>
					!baselineFiles.some(
						(baseline) =>
							fileLocation(file) === fileLocation(baseline) && file.content === baseline.content
					)
			).length +
			removed.length
	);
	const problems = $derived(configurationProblems(envPath, files));
	const envDirty = $derived(envContent !== savedEnv);
	const filesDirty = $derived(
		JSON.stringify(copyFiles(files)) !== JSON.stringify(copyFiles(savedFiles))
	);
	const dirty = $derived(envDirty || filesDirty || explicitFiles !== savedExplicitFiles);
	const destination = $derived(
		selectedFile?.target === 'release_dir' || removedSelection?.target === 'release_dir'
			? 'Release directory · beside deployed assets'
			: directory || 'Watcher installation directory'
	);
	function choose(index: number) {
		selected = index;
		removedSelection = null;
	}
	function changed() {
		onfileschange?.();
	}
	function update(content: string) {
		if (disabled || removedSelection) return;
		if (selectedFile) {
			selectedFile.content = content;
			changed();
		} else envContent = content;
	}
	function add() {
		files = [...files, { file_path: '', target: 'app_dir', content: '' }];
		choose(files.length - 1);
		review = false;
		changed();
	}
	function useActive() {
		files = copyFiles(baselineFiles);
		choose(-1);
		changed();
	}
	function confirm() {
		if (confirmation === 'remove' && selectedFile) {
			files = files.filter((file) => file !== selectedFile);
			choose(-1);
			changed();
		}
		if (confirmation === 'restore') update(original);
		if (confirmation === 'copy') useActive();
		confirmation = null;
	}
	function formatJSON() {
		try {
			update(JSON.stringify(JSON.parse(currentContent), null, 2) + '\n');
		} catch {
			/* Syntax diagnostics stay visible in the editor. */
		}
	}
</script>

<section class="config-workspace" aria-label={`${serviceName} configuration workspace`}>
	<div class="config-context">
		<div class="min-w-0">
			<p class="eyebrow">
				{mode === 'active'
					? 'Active configuration'
					: mode === 'candidate'
						? 'Candidate draft'
						: 'Service configuration'}
			</p>
			<h3 class="mt-1 truncate text-sm font-semibold">
				{serviceName} <span class="mx-1.5 text-muted-foreground">/</span><span
					class="font-mono text-xs text-primary">{context}</span
				>
			</h3>
		</div>
		<div class="flex flex-wrap items-center gap-3">
			<span
				class={['text-xs', dirty ? 'text-amber-400' : 'text-muted-foreground']}
				aria-live="polite">{dirty ? 'Unsaved edits' : 'No unsaved edits'}</span
			>{#if actions}{@render actions()}{/if}
		</div>
	</div>
	<div class="config-body">
		<aside class="config-files" aria-label={`${serviceName} files`}>
			<div class="flex items-center justify-between px-3 pb-3">
				<span class="eyebrow">Files · {files.length + 1}</span><Button.Root
					variant="ghost"
					size="icon-sm"
					aria-label="Add config file"
					onclick={add}
					{disabled}><FilePlus class="size-4" /></Button.Root
				>
			</div>
			<button
				class={['config-file', !selectedFile && !removedSelection && 'config-file-selected']}
				onclick={() => choose(-1)}
				aria-label={envPath || 'Environment'}
				title={envContent !== baselineEnv ? 'Changed from active configuration' : undefined}
				aria-pressed={!selectedFile && !removedSelection}
				><FileCode class="size-4 shrink-0" /><span class="min-w-0 flex-1 truncate"
					>{envPath || 'Environment'}</span
				>{#if envContent !== baselineEnv}<span class="config-change-dot" aria-label="Changed"
					></span>{/if}</button
			>
			{#each files as file, index (file)}
				{@const baseline = baselineFiles.find(
					(entry) => fileLocation(entry) === fileLocation(file)
				)}
				<button
					class={[
						'config-file',
						selectedFile === file && !removedSelection && 'config-file-selected'
					]}
					onclick={() => choose(index)}
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
						class={[
							'config-file text-red-400',
							removedSelection === file && 'config-file-selected'
						]}
						onclick={() => {
							removedSelection = file;
							review = true;
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
						onclick={() => {
							if (explicitFiles && filesDirty) confirmation = 'copy';
							else useActive();
						}}>{explicitFiles ? 'Copy active files' : 'Stage active files'}</Button.Root
					>{/if}
			</div>
		</aside>
		<div class="config-document">
			<div class="config-editor-toolbar">
				<div
					class="inline-flex rounded-md border border-border p-0.5"
					role="group"
					aria-label="Editor view"
				>
					<Button.Root
						variant={review ? 'ghost' : 'secondary'}
						size="xs"
						onclick={() => (review = false)}
						disabled={Boolean(removedSelection)}><Pencil class="size-3" />Edit</Button.Root
					><Button.Root
						variant={review ? 'secondary' : 'ghost'}
						size="xs"
						onclick={() => (review = true)}><Columns2 class="size-3" />Review changes</Button.Root
					>
				</div>
				<div class="ml-auto flex items-center gap-1">
					{#if /\.json$/i.test(currentPath) && !review}<Button.Root
							variant="ghost"
							size="xs"
							onclick={formatJSON}
							{disabled}
						>
							<Braces class="size-3" />Format JSON</Button.Root
						>{/if}{#if !removedSelection}<Button.Root
							variant="ghost"
							size="xs"
							onclick={() => (confirmation = 'restore')}
							disabled={disabled || currentContent === original}
							><RotateCcw class="size-3" />Restore active</Button.Root
						>{#if selectedFile}<Button.Root
								variant="ghost"
								size="xs"
								onclick={() => (confirmation = 'remove')}
								{disabled}
								aria-label={`Remove ${selectedFile.file_path || 'untitled file'}`}
								><Trash2 class="size-3" /></Button.Root
							>{/if}{/if}
				</div>
			</div>
			{#if selectedFile && !removedSelection}<div class="config-file-metadata">
					<label class="min-w-0 flex-1 text-xs"
						><span class="mb-1 block text-muted-foreground">Relative path</span><Input
							class="font-mono text-xs"
							bind:value={selectedFile.file_path}
							oninput={changed}
							aria-label="Config file path"
							{disabled}
							placeholder="settings/appsettings.json"
						/></label
					><label class="text-xs"
						><span class="mb-1 block text-muted-foreground">Destination</span><select
							class="filter-select w-full"
							bind:value={selectedFile.target}
							onchange={changed}
							aria-label="Config file target"
							{disabled}
							><option value="app_dir">Service/app dir</option><option value="release_dir"
								>Release dir</option
							></select
						></label
					>
				</div>{/if}
			<div
				class="flex flex-wrap items-center gap-x-3 gap-y-1 border-b border-border px-4 py-2 text-[10px] text-muted-foreground"
			>
				<span class="font-mono break-all">{currentPath}</span><span class="ml-auto"
					>{destination}</span
				>
			</div>
			{#if removedSelection}<p class="border-b border-border px-4 py-3 text-xs text-amber-400">
					This file is removed from the managed set. Existing files on disk are not deleted by
					removing the entry.
				</p>{/if}
			{#key `${context}:${selected}:${Boolean(removedSelection)}`}
				<CodeEditor
					value={currentContent}
					original={removedSelection?.content ?? original}
					filename={currentPath}
					label={`${serviceName} ${currentPath} contents`}
					readonly={disabled || Boolean(removedSelection)}
					review={review || Boolean(removedSelection)}
					onchange={update}
				/>
			{/key}
			<div class="config-editor-footer">
				<span
					>{review
						? 'Exact text comparison · removed lines in red, additions in green'
						: 'Ctrl / ⌘ + F to search · standard undo and redo'}</span
				><span class="font-mono">{currentContent.split('\n').length} lines</span>
			</div>
		</div>
	</div>
	{#if problems.length}<div class="space-y-1 border-t border-border px-4 py-3" role="status">
			{#each problems as problem (problem)}<p class="text-xs text-amber-400">{problem}</p>{/each}
		</div>{/if}
	<div class="config-notice">
		<Check class="size-3.5 shrink-0" />
		<p>
			{mode === 'candidate'
				? 'Saving prepares this draft only. Active settings and running services change when you deploy the reviewed release.'
				: mode === 'active'
					? 'Applying writes the environment and app-directory files now. Release-directory files are used on the next deployment. Restart the service when required.'
					: 'Configuration is saved with this service. App-directory files are written on save; release-directory files are used when deploying.'}{#if !envPath}<span
					class="ml-1 text-amber-400"
					>Set an environment file path in service settings to write environment content to disk.</span
				>{/if}
		</p>
	</div>
</section>

<Dialog.Root
	open={confirmation !== null}
	onOpenChange={(open) => {
		if (!open) confirmation = null;
	}}
>
	<Dialog.Content
		><Dialog.Header
			><Dialog.Title
				>{confirmation === 'remove'
					? 'Remove this managed file?'
					: confirmation === 'copy'
						? 'Replace the staged file set?'
						: 'Replace with active content?'}</Dialog.Title
			><Dialog.Description
				>{confirmation === 'remove'
					? 'This removes the entry from your editable file set. Review and save to apply this change. Files already on disk are retained.'
					: confirmation === 'copy'
						? 'All staged managed files will be replaced with copies of the active files. Environment edits are kept.'
						: 'Edits to this file will be replaced with the active configuration. Other files keep their edits.'}</Dialog.Description
			></Dialog.Header
		><Dialog.Footer
			><Button.Root variant="outline" onclick={() => (confirmation = null)}
				>Keep editing</Button.Root
			><Button.Root variant="destructive" onclick={confirm}
				>{confirmation === 'remove'
					? 'Remove file'
					: confirmation === 'copy'
						? 'Copy active files'
						: 'Restore active'}</Button.Root
			></Dialog.Footer
		></Dialog.Content
	>
</Dialog.Root>
