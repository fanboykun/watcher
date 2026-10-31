<script lang="ts">
	import { resolve } from '$app/paths';
	import { type Service } from '$lib/api';
	import * as Button from '$lib/components/ui/button';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { webhookDocsHref } from '$lib/webhooks';
	import { BookOpenText, ExternalLink } from '@lucide/svelte';
	import type { createWatcherSetup } from '$lib/features/watchers/setup/setup-controller.svelte';
	let { model }: { model: ReturnType<typeof createWatcherSetup> } = $props();
</script>

<div class="space-y-4">
	<div class="rounded-lg border border-border/70 bg-muted/20 p-4 text-sm text-muted-foreground">
		Webhook delivery is optional. This step only controls watcher-specific endpoint, signing-secret
		override, and event subscriptions.
	</div>

	<div class="rounded-lg border border-border/70 p-4">
		<div class="flex items-center gap-2">
			<Checkbox id="webhookEnabled" bind:checked={model.ui.webhookEnabled} />
			<Label for="webhookEnabled">Enable webhook delivery for this watcher</Label>
		</div>
		<div class="mt-4 grid gap-4 md:grid-cols-2">
			<div class="space-y-2">
				<Label for="webhookURL">Webhook URL</Label>
				<Input
					id="webhookURL"
					bind:value={model.ui.webhookURL}
					placeholder="https://example.com/hooks/watcher"
				/>
				<p class="text-xs text-muted-foreground">Leave empty to inherit the global default URL.</p>
			</div>
			<div class="space-y-2">
				<Label for="webhookSigningSecret">Webhook Signing Secret Override</Label>
				<Input
					id="webhookSigningSecret"
					type="password"
					bind:value={model.ui.webhookSigningSecret}
					disabled={!model.ui.useCustomWebhookSigningSecret}
					placeholder="base64 secret or whsec_..."
				/>
				<div class="mt-2 flex items-center gap-2">
					<Checkbox
						id="useCustomWebhookSigningSecret"
						bind:checked={model.ui.useCustomWebhookSigningSecret}
					/>
					<Label for="useCustomWebhookSigningSecret"
						>Use watcher-specific webhook signing secret</Label
					>
				</div>
				<p class="text-xs text-muted-foreground">
					Use the same signing secret as your receiver. Raw base64 secret material or the
					conventional <code>whsec_...</code> form both work.
				</p>
			</div>
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
			<div class="flex flex-wrap gap-2">
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
</div>
