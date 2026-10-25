<script lang="ts">
	import { api } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import * as Tabs from '$lib/components/ui/tabs';
	import type { createSettings } from '$lib/features/settings/settings-controller.svelte';
	let { model }: { model: ReturnType<typeof createSettings> } = $props();
</script>

<Tabs.Content value="deployment"
	><fieldset disabled={model.agentBusy || model.isSavingPassword}>
		<Card.Root>
			<Card.Header
				><Card.Title>Deployment defaults</Card.Title><Card.Description
					>Inherited by watchers unless overridden in their settings.</Card.Description
				></Card.Header
			>
			<Card.Content
				><div class="grid gap-4 md:grid-cols-2">
					<div class="space-y-2">
						<label class="text-sm text-muted-foreground" for="cfg-environment">Environment</label>
						<Input
							id="cfg-environment"
							bind:value={model.ui.cfgEnvironment}
							aria-describedby="cfg-environment-help"
						/>
						<p id="cfg-environment-help" class="text-xs text-muted-foreground">
							Default environment name reported to GitHub. Individual watchers can override it.
						</p>
					</div>
					<div class="flex flex-wrap items-center gap-2 py-2">
						<Checkbox
							id="cfg-github-deploy-enabled"
							bind:checked={model.ui.cfgGithubDeployEnabled}
							aria-describedby="cfg-github-deploy-enabled-help"
						/>

						<label
							class="text-sm text-muted-foreground select-none"
							for="cfg-github-deploy-enabled"
						>
							Enable GitHub Deployment API
						</label>
						<p id="cfg-github-deploy-enabled-help" class="w-full text-xs text-muted-foreground">
							Report deployment progress to GitHub. Watcher can still deploy when reporting is
							disabled.
						</p>
					</div>
					<div class="space-y-2 md:col-span-2">
						<label class="text-sm text-muted-foreground" for="cfg-github-token">GitHub token</label>
						<Input
							id="cfg-github-token"
							type="password"
							placeholder={model.agentConfig?.github_token_masked || 'not set'}
							bind:value={model.ui.githubTokenInput}
							disabled={model.ui.clearGitHubToken}
							autocomplete="off"
						/>
						<div class="mt-2 flex items-center gap-2">
							<Checkbox id="clear-github-token" bind:checked={model.ui.clearGitHubToken} />
							<label class="text-xs text-muted-foreground select-none" for="clear-github-token">
								Clear existing GitHub token
							</label>
						</div>
						<p class="text-xs text-muted-foreground">Leave blank to keep the current token.</p>
						<details class="text-xs text-muted-foreground">
							<summary class="cursor-pointer">Token permissions</summary>
							<p class="mt-2">
								Private releases need Contents: Read. Deployment reporting also needs Deployments:
								Read and write. Include the target repositories and any required organization
								authorization.
							</p>
						</details>
					</div>
					<div class="space-y-2 md:col-span-2">
						<label class="text-sm text-muted-foreground" for="cfg-api-base-url">API Base URL</label>
						<Input
							id="cfg-api-base-url"
							bind:value={model.ui.cfgAPIBaseURL}
							placeholder="http://192.168.1.100:8080"
							aria-describedby="cfg-api-base-url-help"
						/>
						<p id="cfg-api-base-url-help" class="text-xs text-muted-foreground">
							Dashboard URL reachable by your team. Used for deployment log links in GitHub.
						</p>
					</div>
				</div></Card.Content
			>
		</Card.Root>
	</fieldset></Tabs.Content
>
