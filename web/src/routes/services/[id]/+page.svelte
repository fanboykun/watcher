<!-- eslint-disable svelte/no-navigation-without-resolve -->
<script lang="ts">
	import DetailTabs from '$lib/features/services/detail/detail-tabs.svelte';
	import DetailHeader from '$lib/features/services/detail/detail-header.svelte';
	import UnsavedChanges from '$lib/components/console/unsaved-changes.svelte';
	import RequestError from '$lib/components/request-error.svelte';
	import { message } from '$lib/query/client';
	import ServiceStatus from '$lib/components/service-status.svelte';

	import {
		api,
		iisAppKindLabel,
		isIISService,
		serviceTypeLabel,
		type Service,
		type Watcher
	} from '$lib/api';
	import * as Card from '$lib/components/ui/card';

	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';

	import { ExternalLink, TerminalSquare } from '@lucide/svelte';

	import { resolve } from '$app/paths';

	import RequestLoading from '$lib/components/request-loading.svelte';
	import { createServiceDetail } from '$lib/features/services/detail/detail-controller.svelte';
	const model = createServiceDetail();
</script>

<UnsavedChanges dirty={model.configDirty} label="configuration" />
<div class="space-y-6">
	<DetailHeader {model} />

	{#if !model.ui.showDeleteDialog && !model.ui.confirmServiceAction}<RequestError
			message={model.ui.error || message(model.detail.error)}
			onRetry={!model.service ? model.loadDetail : undefined}
		/>{/if}

	{#if model.ui.actionMsg}
		<div class="rounded-lg border border-blue-500/30 bg-blue-500/10 p-4 text-sm text-blue-400">
			{model.ui.actionMsg}
		</div>
	{/if}

	{#if model.loading}
		<RequestLoading label="Loading service details…" />
	{:else if model.service}
		<ServiceStatus
			service={model.service}
			variant="detail"
			checking={model.checkingStatus}
			onRefresh={() => model.refreshStatus()}
			refreshDisabled={Boolean(model.pendingAction)}
		/>
		<RequestError
			message={model.statusError}
			title="Status could not be refreshed"
			onRetry={() => model.refreshStatus()}
		/>
		<!-- Service Info Card -->
		<Card.Root class="border-border bg-card">
			<Card.Content class="grid min-w-0 grid-cols-2 gap-x-6 gap-y-4 px-6 sm:grid-cols-3">
				<div>
					<p class="text-xs text-muted-foreground">Hosting Mode</p>
					<p class="mt-1 text-sm">{serviceTypeLabel(model.service.service_type)}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground">
						{isIISService(model.service.service_type) ? 'IIS App Kind' : 'Binary'}
					</p>
					<p class="mt-1 font-mono text-sm break-all">
						{isIISService(model.service.service_type)
							? iisAppKindLabel(model.service.iis_app_kind || 'static')
							: model.service.binary_name || '—'}
					</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground">
						{isIISService(model.service.service_type) ? 'IIS App Pool' : 'Env File'}
					</p>
					<p class="mt-1 font-mono text-sm break-all">
						{isIISService(model.service.service_type)
							? model.service.iis_app_pool || '—'
							: model.service.env_file || '—'}
					</p>
				</div>
				{#if isIISService(model.service.service_type)}
					<div>
						<p class="text-xs text-muted-foreground">IIS Site Name</p>
						<p class="mt-1 font-mono text-sm break-all">{model.service.iis_site_name || '—'}</p>
					</div>
				{/if}
				<div>
					<p class="text-xs text-muted-foreground">Health URL</p>
					<p class="mt-1 font-mono text-sm break-all">
						{model.service.health_check_url || model.watcher?.hc_url || 'Not configured'}
					</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground">Install Dir</p>
					<p class="mt-1 font-mono text-sm break-all">{model.watcher?.install_dir ?? '—'}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground">Public URL</p>
					<p class="mt-1 font-mono text-sm break-all">
						{#if model.service.public_url}
							<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
							<Button.Root
								variant="link"
								size="sm"
								href={model.service.public_url}
								data-sveltekit-noscroll
								target="_blank"
								rel="noopener noreferrer"
								class="h-auto min-w-0 justify-start gap-1.5 p-0 text-left text-xs break-all whitespace-normal text-blue-400 hover:underline"
							>
								{model.service.public_url}
								<ExternalLink class="h-3 w-3" />
							</Button.Root>
						{:else}
							—
						{/if}
					</p>
				</div>
			</Card.Content>
		</Card.Root>

		{#if isIISService(model.service.service_type)}
			<details class="rounded-lg border border-border p-4">
				<summary class="cursor-pointer text-sm font-medium">IIS deployment details</summary>
				<div class="mb-2 flex items-center gap-2 font-medium text-blue-400">
					<TerminalSquare class="h-5 w-5" />
					IIS Bootstrap
				</div>
				<p class="mb-4 text-sm text-foreground/80">
					Watcher can now create the IIS app pool and site automatically on first deploy when
					<code>iis_app_pool</code>, <code>iis_site_name</code>, and <code>public_url</code> are
					set. The root application is kept pointed at
					<code>{model.watcher?.install_dir}.urrent</code> on each deploy.
				</p>
				<p class="text-sm text-foreground/80">
					This service is configured as <code
						>{iisAppKindLabel(model.service.iis_app_kind || 'static')}</code
					>. Watcher will choose the IIS managed runtime automatically for that app kind, and if the
					site already exists it will reuse it and refresh the root path and app pool assignment.
				</p>
				<p class="text-sm text-foreground/80">
					Watcher does not install PHP, .NET hosting bundles, or IIS handler mappings. Those
					server-level prerequisites still need to exist before the deployed site can serve traffic
					successfully.
				</p>
			</details>
		{/if}

		<DetailTabs {model} />
	{/if}
</div>

<Dialog.Root
	bind:open={model.ui.showDeleteDialog}
	onOpenChange={(open) => {
		if (model.deleting && !open) model.ui.showDeleteDialog = true;
	}}
>
	<Dialog.Content class="sm:max-w-[420px]" showCloseButton={!model.deleting}>
		<Dialog.Header>
			<Dialog.Title>Delete Service</Dialog.Title>
			<Dialog.Description>
				Delete <span class="font-medium"
					>{model.service?.windows_service_name || 'this service'}</span
				>? This removes it from Watcher.
			</Dialog.Description>
		</Dialog.Header>
		<RequestError message={model.ui.error || message(model.detail.error)} />
		<Dialog.Footer>
			<Button.Root
				variant="outline"
				type="button"
				onclick={() => (model.ui.showDeleteDialog = false)}
				disabled={model.deleting}
			>
				Cancel
			</Button.Root>
			<Button.Root
				type="button"
				class="bg-red-600 text-white hover:bg-red-700"
				onclick={model.deleteService}
				disabled={model.deleting}
			>
				{model.deleting ? 'Deleting...' : 'Delete Service'}
			</Button.Root>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<Dialog.Root
	open={model.ui.confirmServiceAction !== null}
	onOpenChange={(open) => {
		if (!open && !model.pendingAction) model.ui.confirmServiceAction = null;
	}}
>
	<Dialog.Content showCloseButton={!model.pendingAction}>
		<Dialog.Header
			><Dialog.Title
				>{model.ui.confirmServiceAction === 'stop'
					? 'Stop service?'
					: 'Restart service?'}</Dialog.Title
			><Dialog.Description
				>{model.service?.windows_service_name} will {model.ui.confirmServiceAction === 'stop'
					? 'stop serving traffic.'
					: 'briefly stop serving traffic while it restarts.'}</Dialog.Description
			></Dialog.Header
		>
		<RequestError message={model.ui.error || message(model.detail.error)} />
		<Dialog.Footer>
			<Button.Root
				variant="outline"
				disabled={Boolean(model.pendingAction)}
				onclick={() => (model.ui.confirmServiceAction = null)}>Cancel</Button.Root
			>
			<Button.Root
				variant="destructive"
				disabled={Boolean(model.pendingAction)}
				loading={Boolean(model.pendingAction)}
				onclick={async () => {
					const action = model.ui.confirmServiceAction;
					if (!action) return;
					await model.runAction(action, () =>
						action === 'stop' ? api.stopService(model.id) : api.restartService(model.id)
					);
					if (!model.ui.error) model.ui.confirmServiceAction = null;
				}}
				>{model.ui.confirmServiceAction === 'stop'
					? 'Stop service'
					: 'Restart service'}</Button.Root
			>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
