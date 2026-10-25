<script lang="ts">
	import { resolve } from '$app/paths';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import * as Tabs from '$lib/components/ui/tabs';
	import type { createSettings } from '$lib/features/settings/settings-controller.svelte';
	let { model }: { model: ReturnType<typeof createSettings> } = $props();
</script>

<Tabs.Content value="webhooks"
	><fieldset disabled={model.agentBusy || model.isSavingPassword}>
		<Card.Root class="bg-card">
			<Card.Header>
				<Card.Title>Webhook Defaults</Card.Title>
				<Card.Description>
					Watchers inherit these values unless they supply their own URL or signing secret.
				</Card.Description><Button.Root size="sm" variant="outline" href={resolve('/docs/webhooks')}
					>Integration guide</Button.Root
				>
			</Card.Header>
			<Card.Content class="space-y-4">
				{#if model.agentConfig}
					<div class="grid gap-4 md:grid-cols-2">
						<div class="space-y-2 md:col-span-2">
							<label class="text-sm text-muted-foreground" for="cfg-webhook-default-url"
								>Default Webhook URL</label
							>
							<Input
								id="cfg-webhook-default-url"
								bind:value={model.ui.cfgWebhookDefaultURL}
								placeholder="https://example.com/hooks/watcher"
							/>
							<p class="text-xs text-muted-foreground">
								Watchers can override this, but leaving watcher URL blank will inherit this default.
							</p>
						</div>
						<div class="space-y-2 md:col-span-2">
							<label class="text-sm text-muted-foreground" for="cfg-webhook-default-signing-secret">
								Signing secret
							</label>
							<Input
								id="cfg-webhook-default-signing-secret"
								type="password"
								placeholder={model.agentConfig.webhook_default_signing_secret_masked || 'not set'}
								bind:value={model.ui.webhookDefaultSigningSecretInput}
								disabled={model.ui.clearWebhookDefaultSigningSecret}
								autocomplete="off"
							/>
							<div class="mt-2 flex items-center gap-2">
								<Checkbox
									id="clear-webhook-default-signing-secret"
									bind:checked={model.ui.clearWebhookDefaultSigningSecret}
								/>
								<label
									class="text-xs text-muted-foreground select-none"
									for="clear-webhook-default-signing-secret"
								>
									Clear existing default webhook signing secret
								</label>
							</div>
							<p class="text-xs text-muted-foreground">
								Use a Standard Webhooks HMAC signing secret. Raw base64 secret material or the
								conventional <code>whsec_...</code> form both work.
							</p>
						</div>
						<details class="md:col-span-2">
							<summary class="cursor-pointer text-sm font-medium"
								>Delivery, retries and retention</summary
							>
							<div class="mt-4 grid gap-4 md:grid-cols-2">
								<div class="space-y-2">
									<label class="text-sm text-muted-foreground" for="cfg-webhook-timeout-sec"
										>Webhook Timeout (s)</label
									>
									<Input
										id="cfg-webhook-timeout-sec"
										type="number"
										min="1"
										bind:value={model.ui.cfgWebhookTimeoutSec}
										aria-describedby="cfg-webhook-timeout-sec-help"
									/>
									<p id="cfg-webhook-timeout-sec-help" class="text-xs text-muted-foreground">
										Maximum seconds to wait for the receiver to respond to each delivery.
									</p>
								</div>
								<div class="space-y-2">
									<label class="text-sm text-muted-foreground" for="cfg-webhook-retry-schedule-sec">
										Webhook Retry Schedule (seconds CSV)
									</label>
									<Input
										id="cfg-webhook-retry-schedule-sec"
										bind:value={model.ui.cfgWebhookRetryScheduleSec}
										placeholder="0,10,60,300"
										aria-describedby="cfg-webhook-retry-schedule-sec-help"
									/>
									<p id="cfg-webhook-retry-schedule-sec-help" class="text-xs text-muted-foreground">
										Comma-separated retry delays in seconds, for example 0,10,60,300.
									</p>
								</div>
								<div class="flex items-center gap-2 py-2">
									<Checkbox
										id="cfg-webhook-auto-pause-enabled"
										bind:checked={model.ui.cfgWebhookAutoPauseEnabled}
									/>
									<label
										class="text-sm text-muted-foreground select-none"
										for="cfg-webhook-auto-pause-enabled"
									>
										Enable webhook auto-pause
									</label>
								</div>
								<div class="space-y-2">
									<label
										class="text-sm text-muted-foreground"
										for="cfg-webhook-auto-pause-after-failures"
									>
										Auto-pause after failures
									</label>
									<Input
										id="cfg-webhook-auto-pause-after-failures"
										type="number"
										min="1"
										bind:value={model.ui.cfgWebhookAutoPauseAfterFailures}
										aria-describedby="cfg-webhook-auto-pause-after-failures-help"
									/>
									<p
										id="cfg-webhook-auto-pause-after-failures-help"
										class="text-xs text-muted-foreground"
									>
										Failed deliveries allowed before automatic delivery pauses. Used when auto-pause
										is enabled.
									</p>
								</div>
								<div class="space-y-2">
									<label
										class="text-sm text-muted-foreground"
										for="cfg-webhook-event-retention-days"
									>
										Webhook Event Retention (days)
									</label>
									<Input
										id="cfg-webhook-event-retention-days"
										type="number"
										min="1"
										bind:value={model.ui.cfgWebhookEventRetentionDays}
										aria-describedby="cfg-webhook-event-retention-days-help"
									/>
									<p
										id="cfg-webhook-event-retention-days-help"
										class="text-xs text-muted-foreground"
									>
										Days to keep recorded webhook events before cleanup.
									</p>
								</div>
								<div class="space-y-2">
									<label
										class="text-sm text-muted-foreground"
										for="cfg-webhook-delivery-retention-days"
									>
										Webhook Delivery Retention (days)
									</label>
									<Input
										id="cfg-webhook-delivery-retention-days"
										type="number"
										min="1"
										bind:value={model.ui.cfgWebhookDeliveryRetentionDays}
										aria-describedby="cfg-webhook-delivery-retention-days-help"
									/>
									<p
										id="cfg-webhook-delivery-retention-days-help"
										class="text-xs text-muted-foreground"
									>
										Days to keep delivery attempts and their responses before cleanup.
									</p>
								</div>
							</div>
						</details>
					</div>
				{:else}
					<div class="h-20 animate-pulse rounded bg-muted/50"></div>
				{/if}
			</Card.Content>
		</Card.Root>
	</fieldset></Tabs.Content
>
