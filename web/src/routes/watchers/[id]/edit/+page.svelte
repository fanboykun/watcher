<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import {
		api,
		type Service,
		type Watcher
	} from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { webhookDocsHref } from '$lib/webhooks';
	import type { WebhookSelectionState } from '$lib/webhooks';
	import {
		ArrowLeft,
		AlertCircle,
		CheckCircle2,
		BookOpenText,
		ExternalLink,
		Save,
		Send,
		Link as LinkIcon
	} from '@lucide/svelte';
	import ServicesTab from '../components/services-tab.svelte';

	const id = Number(page.params.id);

	let watcher = $state<Watcher | null>(null);
	let error = $state('');
	let success = $state('');
	let saving = $state(false);
	let sendingTest = $state(false);

	let showConfirmDialog = $state(false);
	let confirming = $state(false);
	let confirmTitle = $state('');
	let confirmDescription = $state('');
	let confirmActionLabel = $state('Confirm');
	let confirmActionClass = $state('');
	let confirmAction: (() => Promise<void> | void) | null = null;

	let editInterval = $state(60);
	let editMetadataURL = $state('');
	let editInstallDir = $state('');
	let editReleaseRef = $state('latest');
	let editHcEnabled = $state(false);
	let editHcURL = $state('');
	let editMaxKeptVersions = $state(3);
	let editDeploymentEnvironment = $state('');
	let editGitHubToken = $state('');
	let editUseGlobalToken = $state(false);
	let editWebhookEnabled = $state(false);
	let editWebhookURL = $state('');
	let editWebhookBearerToken = $state('');
	let editUseGlobalWebhookToken = $state(false);
	let webhookSelections = $state<WebhookSelectionState>({
		notify_version_found: false,
		notify_deployment_succeeded: false,
		notify_deployment_failed: false,
		notify_rollback_succeeded: false,
		notify_rollback_failed: false,
		notify_service_health_changed: false
	});

	onMount(() => {
		void loadWatcher();
	});

	async function loadWatcher() {
		try {
			watcher = await api.getWatcher(id);
			syncEditForm();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load watcher';
		}
	}

	function syncEditForm() {
		if (!watcher) return;
		editInterval = watcher.check_interval_sec;
		editMetadataURL = watcher.metadata_url;
		editInstallDir = watcher.install_dir;
		editReleaseRef = watcher.release_ref || 'latest';
		editHcEnabled = watcher.hc_enabled;
		editHcURL = watcher.hc_url;
		editMaxKeptVersions = watcher.max_kept_versions;
		editDeploymentEnvironment = watcher.deployment_environment || '';
		editGitHubToken = '';
		editUseGlobalToken = !watcher.has_github_token;
		editWebhookEnabled = watcher.webhook_enabled;
		editWebhookURL = watcher.webhook_url || '';
		editWebhookBearerToken = '';
		editUseGlobalWebhookToken = !watcher.has_webhook_bearer_token;
		webhookSelections = {
			notify_version_found: watcher.notify_version_found,
			notify_deployment_succeeded: watcher.notify_deployment_succeeded,
			notify_deployment_failed: watcher.notify_deployment_failed,
			notify_rollback_succeeded: watcher.notify_rollback_succeeded,
			notify_rollback_failed: watcher.notify_rollback_failed,
			notify_service_health_changed: watcher.notify_service_health_changed
		};
	}

	async function saveEdit() {
		saving = true;
		error = '';
		success = '';
		try {
			watcher = await api.updateWatcher(id, {
				check_interval_sec: editInterval,
				metadata_url: editMetadataURL,
				release_ref: editReleaseRef.trim() || 'latest',
				deployment_environment: editDeploymentEnvironment,
				github_token: editUseGlobalToken ? '' : editGitHubToken.trim() !== '' ? editGitHubToken : undefined,
				webhook_enabled: editWebhookEnabled,
				webhook_url: editWebhookURL,
				webhook_bearer_token:
					editUseGlobalWebhookToken
						? ''
						: editWebhookBearerToken.trim() !== ''
							? editWebhookBearerToken
							: undefined,
				notify_version_found: webhookSelections.notify_version_found,
				notify_deployment_succeeded: webhookSelections.notify_deployment_succeeded,
				notify_deployment_failed: webhookSelections.notify_deployment_failed,
				notify_rollback_succeeded: webhookSelections.notify_rollback_succeeded,
				notify_rollback_failed: webhookSelections.notify_rollback_failed,
				notify_service_health_changed: webhookSelections.notify_service_health_changed,
				install_dir: editInstallDir,
				hc_enabled: editHcEnabled,
				hc_url: editHcURL,
				max_kept_versions: editMaxKeptVersions
			});
			success = 'Watcher settings saved.';
			syncEditForm();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Save failed';
		} finally {
			saving = false;
		}
	}

	async function sendWebhookTest() {
		sendingTest = true;
		error = '';
		success = '';
		try {
			const res = await api.sendWatcherWebhookTest(id);
			success = res.message;
			watcher = await api.getWatcher(id);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to send webhook test';
		} finally {
			sendingTest = false;
		}
	}

	function openConfirmDialog(opts: {
		title: string;
		description: string;
		actionLabel: string;
		actionClass?: string;
		action: () => Promise<void> | void;
	}) {
		confirmTitle = opts.title;
		confirmDescription = opts.description;
		confirmActionLabel = opts.actionLabel;
		confirmActionClass = opts.actionClass || '';
		confirmAction = opts.action;
		showConfirmDialog = true;
	}

	async function runConfirmAction() {
		if (!confirmAction) return;
		confirming = true;
		try {
			await confirmAction();
			showConfirmDialog = false;
		} finally {
			confirming = false;
		}
	}

	function deleteService(svcId: number, name: string) {
		openConfirmDialog({
			title: 'Delete Service',
			description: `Delete service "${name}"?`,
			actionLabel: 'Delete',
			actionClass: 'bg-red-600 text-white hover:bg-red-700',
			action: async () => {
				try {
					await api.deleteService(id, svcId);
					await loadWatcher();
					success = 'Service deleted.';
				} catch (err) {
					error = err instanceof Error ? err.message : 'Delete failed';
				}
			}
		});
	}
</script>

<div class="space-y-6">
	<div class="flex items-center gap-4">
		<a href={resolve(`/watchers/${id}`)}>
			<Button.Root variant="ghost" size="icon" class="h-8 w-8">
				<ArrowLeft class="h-4 w-4" />
			</Button.Root>
		</a>
		<div class="flex-1">
			<h1 class="text-2xl font-bold tracking-tight">
				{watcher ? `Edit ${watcher.name}` : 'Edit Watcher'}
			</h1>
			{#if watcher}
				<p class="font-mono text-sm text-muted-foreground">{watcher.service_name}</p>
			{/if}
		</div>
		<a href={resolve(`/watchers/${id}?tab=webhooks`)}>
			<Button.Root variant="outline" size="sm">
				<LinkIcon class="mr-2 h-4 w-4" /> Webhook History
			</Button.Root>
		</a>
		<Button.Root size="sm" onclick={saveEdit} disabled={saving}>
			<Save class="mr-2 h-4 w-4" /> {saving ? 'Saving...' : 'Save Changes'}
		</Button.Root>
	</div>

	{#if error}
		<div class="flex items-center rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-400">
			<AlertCircle class="mr-2 h-4 w-4 shrink-0" />
			<span>{error}</span>
		</div>
	{/if}

	{#if success}
		<div class="flex items-center rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-4 text-sm text-emerald-400">
			<CheckCircle2 class="mr-2 h-4 w-4 shrink-0" />
			<span>{success}</span>
		</div>
	{/if}

	{#if watcher}
		<form
			class="space-y-6"
			onsubmit={(event) => {
				event.preventDefault();
				saveEdit();
			}}
		>
			<Card.Root class="border-border bg-card">
				<Card.Header>
					<Card.Title>Watcher Settings</Card.Title>
					<Card.Description>
						Core polling, release, install, and GitHub deployment settings for this watcher.
					</Card.Description>
				</Card.Header>
				<Card.Content class="space-y-4">
					<div class="grid gap-4 sm:grid-cols-2">
						<div class="space-y-2">
							<Label>Name</Label>
							<Input value={watcher.name} disabled />
						</div>
						<div class="space-y-2">
							<Label>Service Name</Label>
							<Input value={watcher.service_name} disabled />
						</div>
					</div>
					<div class="space-y-2">
						<Label for="editMetadataURL">Metadata URL</Label>
						<Input id="editMetadataURL" bind:value={editMetadataURL} />
					</div>
					<div class="grid gap-4 sm:grid-cols-2">
						<div class="space-y-2">
							<Label for="editReleaseRef">Release Ref</Label>
							<Input id="editReleaseRef" bind:value={editReleaseRef} placeholder="latest or v1.2.3" />
							<p class="text-xs text-muted-foreground">
								Use <code>latest</code> to follow new releases, or pin this watcher to a specific release tag.
							</p>
						</div>
						<div class="space-y-2">
							<Label for="editInstallDir">Install Directory</Label>
							<Input id="editInstallDir" bind:value={editInstallDir} />
						</div>
					</div>
					<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
						<div class="space-y-2">
							<Label for="editInterval">Check Interval (s)</Label>
							<Input id="editInterval" type="number" min="10" bind:value={editInterval} />
						</div>
						<div class="space-y-2">
							<Label for="editHcURL">Health Check URL</Label>
							<Input id="editHcURL" bind:value={editHcURL} />
						</div>
						<div class="space-y-2">
							<Label for="editMaxKeptVersions">Max Kept Versions</Label>
							<Input id="editMaxKeptVersions" type="number" min="1" max="10" bind:value={editMaxKeptVersions} />
						</div>
						<div class="space-y-2">
							<Label for="editDeploymentEnvironment">Deployment Environment</Label>
							<Input
								id="editDeploymentEnvironment"
								bind:value={editDeploymentEnvironment}
								placeholder="production"
							/>
						</div>
					</div>
					<div class="flex items-center gap-2 py-1">
						<Checkbox id="editHcEnabled" bind:checked={editHcEnabled} />
						<Label for="editHcEnabled">Enable health checks</Label>
					</div>
					<div class="space-y-2">
						<Label for="editGitHubToken">GitHub Access Token Override</Label>
						<Input
							id="editGitHubToken"
							type="password"
							bind:value={editGitHubToken}
							placeholder="Paste new token to replace override"
							disabled={editUseGlobalToken}
						/>
						<div class="mt-2 flex items-center gap-2">
							<Checkbox id="editUseGlobalToken" bind:checked={editUseGlobalToken} />
							<Label for="editUseGlobalToken">Use global `GITHUB_TOKEN`</Label>
						</div>
						<p class="mt-1 text-xs text-muted-foreground">
							Current: {watcher.has_github_token ? watcher.github_token_masked || 'set' : 'using global token'}
						</p>
					</div>
				</Card.Content>
			</Card.Root>

			<Card.Root class="border-border bg-card" id="webhooks">
				<Card.Header>
					<div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
						<div>
							<Card.Title>Webhook Settings</Card.Title>
							<Card.Description>
								Configure this watcher's endpoint, token override, and event subscriptions.
							</Card.Description>
							<div class="mt-3 flex flex-wrap gap-2">
								<a href={resolve('/docs/webhooks')}>
									<Button.Root type="button" variant="outline" size="sm">
										<BookOpenText class="mr-2 h-4 w-4" />
										Integration Guide
									</Button.Root>
								</a>
								<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
								<a href={webhookDocsHref} target="_blank" rel="noopener noreferrer">
									<Button.Root type="button" variant="outline" size="sm">
										<ExternalLink class="mr-2 h-4 w-4" />
										Repo Docs
									</Button.Root>
								</a>
							</div>
						</div>
						<Button.Root
							type="button"
							variant="outline"
							size="sm"
							onclick={sendWebhookTest}
							disabled={sendingTest}
						>
							<Send class="mr-2 h-4 w-4" /> {sendingTest ? 'Sending...' : 'Send Test Webhook'}
						</Button.Root>
					</div>
				</Card.Header>
				<Card.Content class="space-y-4">
					<div class="rounded-lg border border-border/70 bg-muted/20 p-4 text-sm text-muted-foreground">
						Use this form only for watcher-specific webhook configuration. Delivery history, replay, and pause recovery live in the webhook hub and the watcher webhook tab.
					</div>
					<div class="flex items-center gap-2">
						<Checkbox id="editWebhookEnabled" bind:checked={editWebhookEnabled} />
						<Label for="editWebhookEnabled">Enable webhook delivery for this watcher</Label>
					</div>
					<div class="grid gap-4 sm:grid-cols-2">
						<div class="space-y-2">
							<Label for="editWebhookURL">Webhook URL</Label>
							<Input
								id="editWebhookURL"
								bind:value={editWebhookURL}
								placeholder="https://example.com/hooks/watcher"
							/>
							<p class="text-xs text-muted-foreground">
								Leave empty to inherit the global default URL.
							</p>
						</div>
						<div class="space-y-2">
							<Label for="editWebhookBearerToken">Webhook Bearer Token Override</Label>
							<Input
								id="editWebhookBearerToken"
								type="password"
								bind:value={editWebhookBearerToken}
								placeholder="Paste new token to replace override"
								disabled={editUseGlobalWebhookToken}
							/>
							<div class="mt-2 flex items-center gap-2">
								<Checkbox id="editUseGlobalWebhookToken" bind:checked={editUseGlobalWebhookToken} />
								<Label for="editUseGlobalWebhookToken">Use global default bearer token</Label>
							</div>
							<p class="mt-1 text-xs text-muted-foreground">
								Current:
								{watcher.has_webhook_bearer_token
									? watcher.webhook_bearer_token_masked || 'set'
									: 'using global webhook token'}
							</p>
						</div>
					</div>
					<div class="space-y-3 rounded-lg border border-border/70 p-4">
						<div class="flex items-start justify-between gap-3">
							<div>
								<h4 class="font-medium">Event Subscriptions</h4>
								<p class="text-sm text-muted-foreground">
									Choose which business events this watcher should emit.
								</p>
							</div>
							<a href={resolve('/docs/webhooks')}>
								<Button.Root type="button" variant="outline" size="sm">
									<BookOpenText class="mr-2 h-4 w-4" />
									Guide
								</Button.Root>
							</a>
						</div>
						<div class="grid gap-3 md:grid-cols-2">
							<label class="flex items-center gap-2 text-sm">
								<Checkbox bind:checked={webhookSelections.notify_version_found} />
								<span>Version Found</span>
							</label>
							<label class="flex items-center gap-2 text-sm">
								<Checkbox bind:checked={webhookSelections.notify_deployment_succeeded} />
								<span>Deployment Succeeded</span>
							</label>
							<label class="flex items-center gap-2 text-sm">
								<Checkbox bind:checked={webhookSelections.notify_deployment_failed} />
								<span>Deployment Failed</span>
							</label>
							<label class="flex items-center gap-2 text-sm">
								<Checkbox bind:checked={webhookSelections.notify_rollback_succeeded} />
								<span>Rollback Succeeded</span>
							</label>
							<label class="flex items-center gap-2 text-sm">
								<Checkbox bind:checked={webhookSelections.notify_rollback_failed} />
								<span>Rollback Failed</span>
							</label>
							<label class="flex items-center gap-2 text-sm">
								<Checkbox bind:checked={webhookSelections.notify_service_health_changed} />
								<span>Service Health Changed</span>
							</label>
						</div>
					</div>
				</Card.Content>
			</Card.Root>

			<div class="flex justify-end">
				<Button.Root type="submit" disabled={saving}>
					<Save class="mr-2 h-4 w-4" /> {saving ? 'Saving...' : 'Save Changes'}
				</Button.Root>
			</div>
		</form>

		<Card.Root class="border-border bg-card" id="services">
			<Card.Header>
				<Card.Title>Service Settings</Card.Title>
				<Card.Description>
					Add, edit, or remove the managed services tied to this watcher.
				</Card.Description>
			</Card.Header>
			<Card.Content>
				<ServicesTab
					{watcher}
					createHref={resolve(`/watchers/${id}/services/new`)}
					editHrefBase="/services"
					onDeleteService={deleteService}
				/>
			</Card.Content>
		</Card.Root>
	{/if}
</div>

<Dialog.Root bind:open={showConfirmDialog}>
	<Dialog.Content class="sm:max-w-115">
		<Dialog.Header>
			<Dialog.Title>{confirmTitle}</Dialog.Title>
			<Dialog.Description>{confirmDescription}</Dialog.Description>
		</Dialog.Header>
		<Dialog.Footer>
			<Button.Root variant="outline" type="button" onclick={() => (showConfirmDialog = false)}>
				Cancel
			</Button.Root>
			<Button.Root
				type="button"
				class={confirmActionClass}
				onclick={runConfirmAction}
				disabled={confirming}
			>
				{confirming ? 'Working...' : confirmActionLabel}
			</Button.Root>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
