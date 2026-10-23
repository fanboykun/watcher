<script lang="ts">
	import type { Service, ServiceConfigFile } from '$lib/api';
	import { isIISService } from '$lib/api';
	import RequestError from '$lib/components/request-error.svelte';
	import ConfigWorkspace from '$lib/features/configuration/config-workspace.svelte';
	import { configurationProblems, configFingerprint } from '$lib/features/configuration/model';
	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Save, RefreshCw, RotateCcw } from '@lucide/svelte';
	let {
		service,
		envContent = $bindable(''),
		configFiles = $bindable([]),
		baselineEnv,
		baselineFiles,
		directory = '',
		savingEnv = false,
		conflict = false,
		saveError = '',
		onSaveEnv,
		onSaveAndRestart,
		onReset
	}: {
		service: Service;
		envContent: string;
		configFiles: ServiceConfigFile[];
		baselineEnv: string;
		baselineFiles: ServiceConfigFile[];
		directory?: string;
		savingEnv?: boolean;
		conflict?: boolean;
		saveError?: string;
		onSaveEnv: () => Promise<boolean>;
		onSaveAndRestart: () => Promise<boolean>;
		onReset: () => void;
	} = $props();
	let confirmation = $state<'apply' | 'restart' | 'reset' | null>(null);
	let opener: HTMLElement | null = null;
	function request(kind: typeof confirmation, event: MouseEvent) {
		opener = event.currentTarget as HTMLElement;
		confirmation = kind;
	}
	const dirty = $derived(
		configFingerprint(envContent, configFiles) !== configFingerprint(baselineEnv, baselineFiles)
	);
	const problems = $derived(configurationProblems(service.env_file, configFiles));
	async function confirm() {
		if (confirmation === 'reset') {
			onReset();
			confirmation = null;
			return;
		}
		const done = confirmation === 'restart' ? await onSaveAndRestart() : await onSaveEnv();
		if (done) confirmation = null;
	}
</script>

{#if conflict}<p
		role="alert"
		class="mb-4 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-sm text-amber-300"
	>
		Active configuration changed on the server. Your edits are preserved. Review them before
		applying, or use “Reload active” to load the latest saved configuration.
	</p>{/if}
<ConfigWorkspace
	serviceName={service.windows_service_name || service.binary_name}
	context="Active files"
	mode="active"
	{directory}
	envPath={service.env_file}
	bind:envContent
	bind:files={configFiles}
	{baselineEnv}
	{baselineFiles}
	savedEnv={baselineEnv}
	savedFiles={baselineFiles}
	disabled={savingEnv}
>
	{#snippet actions()}
		<Button.Root
			variant="ghost"
			size="sm"
			onclick={(event) => request('reset', event)}
			disabled={savingEnv || (!dirty && !conflict)}
			><RotateCcw class="size-3.5" />{conflict ? 'Reload active' : 'Discard edits'}</Button.Root
		>
		<Button.Root
			variant="outline"
			size="sm"
			onclick={(event) => request('apply', event)}
			disabled={savingEnv || !dirty || problems.length > 0}
			><Save class="size-3.5" />Apply changes</Button.Root
		>
		{#if !isIISService(service.service_type)}<Button.Root
				size="sm"
				onclick={(event) => request('restart', event)}
				disabled={savingEnv || !dirty || problems.length > 0}
				><RefreshCw class="size-3.5" />Apply and restart</Button.Root
			>{/if}
	{/snippet}
</ConfigWorkspace>

<Dialog.Root
	open={confirmation !== null}
	onOpenChange={(open) => {
		if (!open && !savingEnv) confirmation = null;
	}}
>
	<Dialog.Content
		showCloseButton={!savingEnv}
		onCloseAutoFocus={(event) => {
			if (opener?.isConnected) {
				event.preventDefault();
				// Applying disables the original action when the draft becomes clean.
				const target = opener
					.closest('section')
					?.querySelector<HTMLButtonElement>('button[aria-pressed="true"]');
				if (opener.matches(':disabled')) target?.focus({ preventScroll: true });
				else opener.focus({ preventScroll: true });
			}
		}}
	>
		<Dialog.Header
			><Dialog.Title
				>{confirmation === 'reset'
					? 'Discard local configuration edits?'
					: confirmation === 'restart'
						? 'Apply configuration and restart?'
						: 'Apply active configuration?'}</Dialog.Title
			><Dialog.Description
				>{confirmation === 'reset'
					? 'Replace the editable files with the latest configuration loaded from Watcher.'
					: `${service.windows_service_name}: environment and app-directory files are written immediately. Release-directory files are saved for the next deployment.`}{#if confirmation === 'restart'}
					The service will briefly stop serving traffic while it restarts.{/if}</Dialog.Description
			></Dialog.Header
		>
		{#if conflict && confirmation !== 'reset'}<p class="text-sm text-amber-300">
				The server has newer active configuration. Applying will replace it with your reviewed
				edits.
			</p>{/if}
		<RequestError message={saveError} />
		<Dialog.Footer
			><Button.Root variant="outline" disabled={savingEnv} onclick={() => (confirmation = null)}
				>Keep editing</Button.Root
			><Button.Root
				variant={confirmation === 'reset' ? 'destructive' : 'default'}
				disabled={savingEnv}
				loading={savingEnv}
				onclick={confirm}
				>{confirmation === 'reset'
					? 'Discard edits'
					: confirmation === 'restart'
						? 'Apply and restart'
						: 'Apply changes'}</Button.Root
			></Dialog.Footer
		>
	</Dialog.Content>
</Dialog.Root>
