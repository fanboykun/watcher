<script lang="ts">
	import { message } from '$lib/query/client';
	import * as Tabs from '$lib/components/ui/tabs';
	import { resolve } from '$app/paths';
	import { goto } from '$app/navigation';
	import OverviewTab from '$lib/features/watchers/components/overview-tab.svelte';
	import ServicesTab from '$lib/features/watchers/components/services-tab.svelte';
	import DeploysTab from '$lib/features/watchers/components/deploys-tab.svelte';
	import VersionsTab from '$lib/features/watchers/components/versions-tab.svelte';
	import PollingTab from '$lib/features/watchers/components/polling-tab.svelte';
	import WebhooksTab from '$lib/features/watchers/components/webhooks-tab.svelte';
	import CandidateTab from '$lib/features/watchers/components/candidate-tab.svelte';
	import ReleasesTab from '$lib/features/watchers/components/releases-tab.svelte';
	import DownloadedVersions from '$lib/features/watchers/components/downloaded-versions.svelte';
	import type { createWatcherDetail } from '$lib/features/watchers/detail/detail-controller.svelte';
	let { model }: { model: ReturnType<typeof createWatcherDetail> } = $props();
</script>

{#if model.watcher}
	<Tabs.Root
		bind:value={model.ui.activeTab}
		onValueChange={(v) => {
			if (v) {
				if (v === 'versions')
					void model.loadVersions().catch((cause) => {
						model.ui.error = cause instanceof Error ? cause.message : 'Could not load versions';
					});
				goto(resolve(`/watchers/[id]?tab=${v}`, { id: String(model.id) }), {
					replaceState: true,
					keepFocus: true,
					noScroll: true
				}).catch(() => {});
			}
		}}
	>
		<Tabs.List>
			<Tabs.Trigger value="overview">Overview</Tabs.Trigger>
			<Tabs.Trigger value="services">Services ({model.watcher.services.length})</Tabs.Trigger>
			<Tabs.Trigger value="releases">GitHub releases</Tabs.Trigger>
			<Tabs.Trigger value="candidates" class="relative">
				Candidates
				{#if model.reviewableCandidate}
					<span class="ml-1.5 flex h-2 w-2 rounded-full bg-primary"></span>
				{/if}
			</Tabs.Trigger>
			<Tabs.Trigger value="deploys">Deploys ({model.deployTotal})</Tabs.Trigger>
			<Tabs.Trigger value="versions"
				>Versions ({model.versions.length + model.downloads.length})</Tabs.Trigger
			>
			<Tabs.Trigger value="polling">Polls</Tabs.Trigger>
			<Tabs.Trigger value="webhooks">Webhooks ({model.deliveryTotal})</Tabs.Trigger>
		</Tabs.List>

		<Tabs.Content value="overview" class="mt-4">
			<OverviewTab watcher={model.watcher} />
		</Tabs.Content>

		<Tabs.Content value="services" class="mt-4">
			<ServicesTab
				watcher={model.watcher}
				readonly={true}
				manageHref={resolve(`/watchers/${model.id}/edit#services`)}
			/>
		</Tabs.Content>

		<Tabs.Content value="releases" class="mt-4">
			{#if model.ui.activeTab === 'releases'}
				{#key model.id}
					<ReleasesTab
						watcher={model.watcher}
						onDownloaded={model.loadVersions}
						onNavigate={async (tab) => {
							// The release operation already awaited its invalidation of this watcher.
							model.ui.activeTab = tab;
							await goto(resolve(`/watchers/[id]?tab=${tab}`, { id: String(model.id) }), {
								replaceState: true,
								noScroll: true
							});
						}}
					/>
				{/key}
			{/if}
		</Tabs.Content>

		<Tabs.Content value="candidates" class="mt-4">
			<CandidateTab
				watcher={model.watcher}
				busy={model.actionBusy}
				onApprove={(version, catalogId) =>
					model.runPending('approve', () => model.approveRelease(version, catalogId))}
				onDiscard={model.discardRelease}
				onRefreshWatcher={async () => {
					await model.fetchWatcher();
				}}
			/>
		</Tabs.Content>

		<Tabs.Content value="deploys" class="mt-4">
			<DeploysTab
				deploys={model.deploys}
				bind:deployPage={model.ui.deployPage}
				bind:deployPageSize={model.ui.deployPageSize}
				deployTotal={model.deployTotal}
				paging={model.deploysPaging}
				onPageChange={(p) => {
					model.ui.deployPage = p;
				}}
				onPageSizeChange={(size) => {
					model.ui.deployPageSize = size;
					model.ui.deployPage = 1;
				}}
				watcherId={model.id}
			/>
		</Tabs.Content>

		<Tabs.Content value="versions" class="mt-4 space-y-4">
			<DownloadedVersions
				downloads={model.downloads}
				busy={model.actionBusy}
				onConfigure={(download) =>
					model.runPending('catalog_candidate', () => model.configureDownload(download))}
				onDeploy={model.deployDownload}
			/>
			<VersionsTab
				busy={model.actionBusy}
				versions={model.versions}
				onRollback={model.openRollbackDialog}
				onDeleteVersion={model.deleteVersion}
			/>
		</Tabs.Content>

		<Tabs.Content value="polling" class="mt-4">
			<PollingTab
				watcherId={model.id}
				polls={model.polls}
				bind:pollPage={model.ui.pollPage}
				pollPageSize={model.ui.pollPageSize}
				pollTotal={model.pollTotal}
				bind:pollStatus={model.ui.pollStatus}
				onPageChange={(p) => {
					model.ui.pollPage = p;
				}}
				onStatusChange={(status) => {
					model.ui.pollStatus = status;
					model.ui.pollPage = 1;
				}}
			/>
		</Tabs.Content>

		<Tabs.Content value="webhooks" class="mt-4">
			<WebhooksTab
				watcher={model.watcher}
				deliveries={model.webhookDeliveries}
				bind:deliveryPage={model.ui.deliveryPage}
				bind:deliveryPageSize={model.ui.deliveryPageSize}
				deliveryTotal={model.deliveryTotal}
				onPageChange={(p) => {
					model.ui.deliveryPage = p;
				}}
				onPageSizeChange={(size) => {
					model.ui.deliveryPageSize = size;
					model.ui.deliveryPage = 1;
				}}
				busy={model.actionBusy}
				onSendTest={() => model.runPending('webhook-test', model.sendWebhookTest)}
				onResume={() => model.runPending('webhook-resume', () => model.resumeWebhook(false))}
				onResumeReplay={() => model.runPending('webhook-replay', () => model.resumeWebhook(true))}
			/>
		</Tabs.Content>
	</Tabs.Root>
{/if}
