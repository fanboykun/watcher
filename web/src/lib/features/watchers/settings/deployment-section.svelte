<script lang="ts">
	import * as RadioGroup from '$lib/components/ui/radio-group';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import type { createWatcherSettings } from '$lib/features/watchers/settings/settings-controller.svelte';
	let { model }: { model: ReturnType<typeof createWatcherSettings> } = $props();
</script>

{#if model.watcher}
	<Tabs.Content value="deployment"
		><Card.Root
			><Card.Header
				><Card.Title>Deployment</Card.Title><Card.Description
					>Control polling, deployment policy and GitHub credentials.</Card.Description
				></Card.Header
			><Card.Content class="space-y-4"
				><div class="grid gap-4 sm:grid-cols-2">
					<div class="space-y-2">
						<Label for="editInstallDir">Install Directory</Label>
						<Input
							id="editInstallDir"
							value={model.watcher.install_dir}
							readonly
							aria-describedby="editInstallDir-help"
						/>
						<p id="editInstallDir-help" class="text-xs text-muted-foreground">
							Fixed when this pool is created. Changing its location requires moving its releases,
							snapshots, and service paths through a managed relocation.
						</p>
					</div>
					<div class="space-y-2">
						<Label for="editInterval">Check Interval (s)</Label>
						<Input
							id="editInterval"
							type="number"
							min="10"
							bind:value={model.ui.editInterval}
							aria-describedby="editInterval-help"
						/>
						<p id="editInterval-help" class="text-xs text-muted-foreground">
							Seconds between release checks. Minimum: 10 seconds.
						</p>
					</div>
					<div class="space-y-2">
						<Label for="editMaxKeptVersions">Max Kept Versions</Label>
						<Input
							id="editMaxKeptVersions"
							type="number"
							min="1"
							max="10"
							bind:value={model.ui.editMaxKeptVersions}
							aria-describedby="editMaxKeptVersions-help"
						/>
						<p id="editMaxKeptVersions-help" class="text-xs text-muted-foreground">
							Number of release versions to retain for rollback. Older versions are cleaned up after
							deployment.
						</p>
					</div>
					{#if model.ui.editSourceType === 'github'}<div class="space-y-2">
							<Label for="editDeploymentEnvironment">Deployment Environment</Label>
							<Input
								id="editDeploymentEnvironment"
								bind:value={model.ui.editDeploymentEnvironment}
								placeholder="production"
								aria-describedby="editDeploymentEnvironment-help"
							/>
							<p id="editDeploymentEnvironment-help" class="text-xs text-muted-foreground">
								GitHub environment for this watcher. Leave blank to inherit the global environment.
							</p>
						</div>{/if}
				</div>
				<div class="space-y-2 rounded-lg border border-border/70 bg-muted/20 p-4">
					<Label class="text-sm font-medium">Deployment Policy</Label>
					<RadioGroup.Root
						value={model.ui.editAutoDeploy ? 'automatic' : 'manual'}
						onValueChange={(value) => {
							model.ui.editAutoDeploy = value === 'automatic';
						}}
						aria-label="Deployment policy"
						class="mt-2"
					>
						<Label for="deploy-automatic" class="flex cursor-pointer items-start gap-3"
							><RadioGroup.Item id="deploy-automatic" value="automatic" class="mt-1" />
							<div>
								<p class="text-sm font-medium">Automatic deployment</p>
								<p class="text-xs text-muted-foreground">Deploy new releases when detected.</p>
							</div></Label
						>
						<Label for="deploy-manual" class="flex cursor-pointer items-start gap-3"
							><RadioGroup.Item id="deploy-manual" value="manual" class="mt-1" />
							<div>
								<p class="text-sm font-medium">Manual approval</p>
								<p class="text-xs text-muted-foreground">
									Review release candidates before deploying.
								</p>
							</div></Label
						>
					</RadioGroup.Root>
				</div>
				{#if model.ui.editSourceType === 'github'}<div class="space-y-2">
						<Label for="editGitHubToken">GitHub Access Token Override</Label>
						<Input
							id="editGitHubToken"
							type="password"
							bind:value={model.ui.editGitHubToken}
							placeholder="Paste new token to replace override"
							disabled={model.ui.editUseGlobalToken}
							aria-describedby="editGitHubToken-help"
						/>
						<p id="editGitHubToken-help" class="text-xs text-muted-foreground">
							Leave blank to keep the saved override. Select Use global GitHub token to remove the
							override.
						</p>
						<div class="mt-2 flex items-center gap-2">
							<Checkbox id="editUseGlobalToken" bind:checked={model.ui.editUseGlobalToken} />
							<Label for="editUseGlobalToken">Use global GitHub token</Label>
						</div>
						<p class="mt-1 text-xs text-muted-foreground">
							Current: {model.watcher.has_github_token
								? model.watcher.github_token_masked || 'set'
								: 'using global token'}
						</p>
					</div>{/if}</Card.Content
			></Card.Root
		></Tabs.Content
	>
{/if}
