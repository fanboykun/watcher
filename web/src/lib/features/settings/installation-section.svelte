<script lang="ts">
	import { api } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import * as Tabs from '$lib/components/ui/tabs';
	import type { createSettings } from '$lib/features/settings/settings-controller.svelte';
	let { model }: { model: ReturnType<typeof createSettings> } = $props();
</script>

<Tabs.Content value="installation"
	><fieldset disabled={model.agentBusy || model.isSavingPassword}>
		<Card.Root>
			<Card.Header
				><Card.Title>Agent installation</Card.Title><Card.Description
					>Network, storage and Windows service settings.</Card.Description
				></Card.Header
			>
			<Card.Content
				><div class="grid gap-4 md:grid-cols-2">
					<div class="space-y-2">
						<label class="text-sm text-muted-foreground" for="cfg-api-port">API Port</label>
						<Input
							id="cfg-api-port"
							bind:value={model.ui.cfgAPIPort}
							aria-describedby="cfg-api-port-help"
						/>
						<p id="cfg-api-port-help" class="text-xs text-muted-foreground">
							Port used by the dashboard and API. Requires an agent restart; reconnect using the new
							port.
						</p>
					</div>
					<div class="space-y-2 md:col-span-2">
						<label class="text-sm text-muted-foreground" for="cfg-web-assets-path"
							>Web Assets / Base Path</label
						>
						<Input
							id="cfg-web-assets-path"
							bind:value={model.ui.cfgWebAssetsPath}
							placeholder="/watcher"
						/>
						<p class="text-xs text-muted-foreground">
							Subpath prefix when hosting behind a reverse proxy (e.g. <code>/watcher</code>
							for
							<code>https://domain.co.id/watcher</code>). Leave empty if serving from root.
						</p>
					</div>
					<div class="space-y-2 md:col-span-2">
						<label class="text-sm text-muted-foreground" for="cfg-watcher-repo-url"
							>Watcher Repo URL</label
						>
						<Input
							id="cfg-watcher-repo-url"
							bind:value={model.ui.cfgWatcherRepoURL}
							aria-describedby="cfg-watcher-repo-url-help"
						/>
						<p id="cfg-watcher-repo-url-help" class="text-xs text-muted-foreground">
							Where updates to the Watcher agent itself come from: a GitHub repository, or an
							internal release feed (URL or file share path) listing a <code>watcher</code> service.
						</p>
					</div>
					<div class="space-y-2 md:col-span-2">
						<label class="text-sm text-muted-foreground" for="cfg-watcher-service-name"
							>Watcher Service Name</label
						>
						<Input
							id="cfg-watcher-service-name"
							bind:value={model.ui.cfgWatcherServiceName}
							aria-describedby="cfg-watcher-service-name-help"
						/>
						<p id="cfg-watcher-service-name-help" class="text-xs text-muted-foreground">
							Installed Windows service name used when restarting or updating this agent.
						</p>
					</div>
					<div class="space-y-2 md:col-span-2">
						<label class="text-sm text-muted-foreground" for="cfg-nssm-path">NSSM Path</label>
						<Input
							id="cfg-nssm-path"
							bind:value={model.ui.cfgNssmPath}
							aria-describedby="cfg-nssm-path-help"
						/>
						<p id="cfg-nssm-path-help" class="text-xs text-muted-foreground">
							Full path to nssm.exe on the machine running Watcher.
						</p>
					</div>
					<div class="space-y-2 md:col-span-2">
						<label class="text-sm text-muted-foreground" for="cfg-log-dir">Log Directory</label>
						<Input
							id="cfg-log-dir"
							bind:value={model.ui.cfgLogDir}
							aria-describedby="cfg-log-dir-help"
						/>
						<p id="cfg-log-dir-help" class="text-xs text-muted-foreground">
							Directory for agent log files on the Watcher machine. Restart the agent after changing
							it.
						</p>
					</div>
					<div class="space-y-2 md:col-span-2">
						<label class="text-sm text-muted-foreground" for="cfg-db-path">Database Path</label>
						<Input
							id="cfg-db-path"
							bind:value={model.ui.cfgDBPath}
							aria-describedby="cfg-db-path-help"
						/>
						<p id="cfg-db-path-help" class="text-xs text-muted-foreground">
							Path to the SQLite database. Changing it selects a different database; it does not
							move existing data. Requires a restart.
						</p>
					</div>
				</div></Card.Content
			>
		</Card.Root>
	</fieldset></Tabs.Content
>
