<script lang="ts">
	import RequestError from '$lib/components/request-error.svelte';
	import { message } from '$lib/query/client';
	import { api } from '$lib/api';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Button from '$lib/components/ui/button';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import HealthTab from '$lib/features/services/components/health-tab.svelte';
	import DeploysTab from '$lib/features/services/components/deploys-tab.svelte';
	import LogsTab from '$lib/features/services/components/logs-tab.svelte';
	import EnvTab from '$lib/features/services/components/env-tab.svelte';
	import RequestLoading from '$lib/components/request-loading.svelte';
	import type { createServiceDetail } from '$lib/features/services/detail/detail-controller.svelte';
	let { model }: { model: ReturnType<typeof createServiceDetail> } = $props();
</script>

{#if model.service}
	<Tabs.Root
		bind:value={model.ui.activeTab}
		onValueChange={(v) => {
			if (v) {
				if (v === 'candidates') {
					void model.openCandidates();
					return;
				}
				goto(resolve(`/services/[id]?tab=${v}`, { id: String(model.id) }), {
					replaceState: true,
					keepFocus: true,
					noScroll: true
				}).catch(() => {});
			}
		}}
	>
		<Tabs.List>
			<Tabs.Trigger value="health">Health</Tabs.Trigger>
			<Tabs.Trigger value="logs">Logs</Tabs.Trigger>
			<Tabs.Trigger value="env">Files</Tabs.Trigger>
			<Tabs.Trigger value="candidates">Candidates</Tabs.Trigger>
			<Tabs.Trigger value="deploys">Deploys ({model.deployTotal})</Tabs.Trigger>
		</Tabs.List>

		<!-- Health History -->
		<Tabs.Content value="health" class="mt-4">
			<RequestError message={model.healthError} onRetry={model.loadHealthHistory} />
			{#if model.healthLoading && model.healthHistory.length === 0}<RequestLoading
					label="Loading health history…"
				/>{:else}<HealthTab healthHistory={model.healthHistory} />{/if}
		</Tabs.Content>

		<!-- Logs -->
		<Tabs.Content value="logs" class="mt-4">
			<LogsTab
				loading={model.logsLoading}
				logLines={model.logLines}
				logError={model.logError}
				bind:logType={model.ui.logType}
				bind:logCount={model.ui.logCount}
				onLoadLogs={model.loadLogs}
			/>
		</Tabs.Content>

		<!-- Environment -->
		<Tabs.Content value="env" class="mt-4">
			<EnvTab
				service={model.service}
				baselineEnv={model.ui.baselineEnv}
				baselineFiles={model.ui.baselineFiles}
				directory={model.watcher?.install_dir}
				conflict={model.ui.configConflict}
				saveError={model.ui.error}
				onReset={() => model.resetConfiguration()}
				bind:envContent={model.ui.envContent}
				bind:configFiles={model.ui.configFiles}
				savingEnv={model.savingEnv || model.restartingEnv || model.actionBusy}
				onSaveEnv={async () => {
					return await model.saveEnv();
				}}
				onSaveAndRestart={async () => {
					return model.restartMutation.mutateAsync({
						name: 'apply-restart',
						run: async () => {
							if (!(await model.saveEnv())) return false;
							await model.runAction('restart', () => api.restartService(model.id));
							return !model.ui.error;
						}
					});
				}}
			/>
		</Tabs.Content>

		<!-- Deploys -->
		<Tabs.Content value="deploys" class="mt-4">
			<RequestError message={model.deploysError} onRetry={model.loadDeploys} />
			{#if model.deploysLoading}<RequestLoading label="Loading deployments…" />{:else}
				<DeploysTab
					deploys={model.deploys}
					bind:deployPage={model.ui.deployPage}
					bind:deployPageSize={model.ui.deployPageSize}
					deployTotal={model.deployTotal}
					watcher={model.watcher}
					onLoadDeploys={model.loadDeploys}
				/>
			{/if}
		</Tabs.Content>

		<Tabs.Content value="candidates" class="mt-4">
			<p class="text-sm text-muted-foreground">
				Prepare environment and managed file changes together in this watcher's deployment
				candidates.
			</p>
			<Button.Root class="mt-3" href={model.candidateURL}>Open deployment candidates</Button.Root>
		</Tabs.Content>
	</Tabs.Root>
{/if}
