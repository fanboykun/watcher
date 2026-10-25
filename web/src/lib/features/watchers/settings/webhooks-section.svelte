<script lang="ts">
	import * as Tabs from '$lib/components/ui/tabs';
	import { resolve } from '$app/paths';
	import { type Service } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Send } from '@lucide/svelte';
	import type { createWatcherSettings } from '$lib/features/watchers/settings/settings-controller.svelte';
	let { model }: { model: ReturnType<typeof createWatcherSettings> } = $props();
</script>

{#if model.watcher}
	<Tabs.Content value="webhooks"
		><Card.Root class="border-border bg-card" id="webhooks">
			<Card.Header>
				<div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
					<div>
						<Card.Title>Webhook Settings</Card.Title>
						<Card.Description>
							Configure this watcher's endpoint, signing-secret override, and event subscriptions.
						</Card.Description>
						<div class="mt-3 flex flex-wrap gap-2">
							<Button.Root href={resolve('/docs/webhooks')} variant="outline" size="sm"
								>Integration guide</Button.Root
							>
						</div>
					</div>
					<Button.Root
						type="button"
						variant="outline"
						size="sm"
						onclick={model.sendWebhookTest}
						disabled={model.sendingTest || model.saving || model.confirming}
					>
						<Send class="mr-2 h-4 w-4" />
						{model.sendingTest ? 'Sending...' : 'Send Test Webhook'}
					</Button.Root>
				</div>
			</Card.Header>
			<Card.Content class="space-y-4">
				<div
					class="rounded-lg border border-border/70 bg-muted/20 p-4 text-sm text-muted-foreground"
				>
					Use this form only for watcher-specific webhook configuration. Delivery history, replay,
					and pause recovery live in the webhook hub and the watcher webhook tab.
				</div>
				<div class="flex items-center gap-2">
					<Checkbox id="editWebhookEnabled" bind:checked={model.ui.editWebhookEnabled} />
					<Label for="editWebhookEnabled">Enable webhook delivery for this watcher</Label>
				</div>
				<div class="grid gap-4 sm:grid-cols-2">
					<div class="space-y-2">
						<Label for="editWebhookURL">Webhook URL</Label>
						<Input
							id="editWebhookURL"
							bind:value={model.ui.editWebhookURL}
							placeholder="https://example.com/hooks/watcher"
						/>
						<p class="text-xs text-muted-foreground">
							Leave empty to inherit the global default URL.
						</p>
					</div>
					<div class="space-y-2">
						<Label for="editWebhookSigningSecret">Webhook Signing Secret Override</Label>
						<Input
							id="editWebhookSigningSecret"
							type="password"
							bind:value={model.ui.editWebhookSigningSecret}
							placeholder="Paste new base64 or whsec_... secret to replace override"
							disabled={model.ui.editUseGlobalWebhookToken}
						/>
						<div class="mt-2 flex items-center gap-2">
							<Checkbox
								id="editUseGlobalWebhookToken"
								bind:checked={model.ui.editUseGlobalWebhookToken}
							/>
							<Label for="editUseGlobalWebhookToken">Use global default signing secret</Label>
						</div>
						<p class="mt-1 text-xs text-muted-foreground">
							Current:
							{model.watcher.has_webhook_signing_secret
								? model.watcher.webhook_signing_secret_masked || 'set'
								: 'using global webhook signing secret'}
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
					</div>
					<div class="grid gap-3 md:grid-cols-2">
						<label class="flex items-center gap-2 text-sm">
							<Checkbox bind:checked={model.ui.webhookSelections.notify_version_found} />
							<span>Version Found</span>
						</label>
						<label class="flex items-center gap-2 text-sm">
							<Checkbox bind:checked={model.ui.webhookSelections.notify_deployment_succeeded} />
							<span>Deployment Succeeded</span>
						</label>
						<label class="flex items-center gap-2 text-sm">
							<Checkbox bind:checked={model.ui.webhookSelections.notify_deployment_failed} />
							<span>Deployment Failed</span>
						</label>
						<label class="flex items-center gap-2 text-sm">
							<Checkbox bind:checked={model.ui.webhookSelections.notify_rollback_succeeded} />
							<span>Rollback Succeeded</span>
						</label>
						<label class="flex items-center gap-2 text-sm">
							<Checkbox bind:checked={model.ui.webhookSelections.notify_rollback_failed} />
							<span>Rollback Failed</span>
						</label>
						<label class="flex items-center gap-2 text-sm">
							<Checkbox bind:checked={model.ui.webhookSelections.notify_service_health_changed} />
							<span>Service Health Changed</span>
						</label>
					</div>
				</div>
			</Card.Content>
		</Card.Root></Tabs.Content
	>
{/if}
