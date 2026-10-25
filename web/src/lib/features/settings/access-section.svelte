<script lang="ts">
	import RequestError from '$lib/components/request-error.svelte';
	import { message } from '$lib/query/client';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { LockKeyhole } from '@lucide/svelte';
	import * as Tabs from '$lib/components/ui/tabs';
	import type { createSettings } from '$lib/features/settings/settings-controller.svelte';
	let { model }: { model: ReturnType<typeof createSettings> } = $props();
</script>

<Tabs.Content value="access">
	<Card.Root class="bg-card">
		<Card.Header>
			<Card.Title class="flex items-center gap-2">
				<LockKeyhole class="h-4 w-4" /> Dashboard Password
			</Card.Title>
			<Card.Description>Change the password used for the dashboard and API.</Card.Description>
		</Card.Header>
		<Card.Content class="space-y-4">
			<RequestError message={model.ui.passwordError} />
			{#if model.ui.passwordSuccess}<p role="status" class="text-sm text-green-400">
					{model.ui.passwordSuccess}
				</p>{/if}
			<form
				onsubmit={(event) => {
					event.preventDefault();
					void model.savePassword();
				}}
			>
				<fieldset disabled={model.isSavingPassword || model.agentBusy} class="space-y-4">
					{#if model.authStatus?.using_default_password}
						<div
							class="rounded-md border border-amber-500/30 bg-amber-500/10 p-3 text-sm text-amber-300"
						>
							The dashboard is still using the default password. Change it before exposing Watcher
							beyond a trusted machine.
						</div>
					{/if}

					<div class="grid gap-4 md:grid-cols-3">
						<div class="space-y-2">
							<label class="text-sm text-muted-foreground" for="current-password"
								>Current Password</label
							>
							<Input
								id="current-password"
								type="password"
								bind:value={model.ui.currentPassword}
								autocomplete="current-password"
							/>
						</div>
						<div class="space-y-2">
							<label class="text-sm text-muted-foreground" for="new-password">New Password</label>
							<Input
								id="new-password"
								type="password"
								bind:value={model.ui.newPassword}
								autocomplete="new-password"
							/>
						</div>
						<div class="space-y-2">
							<label class="text-sm text-muted-foreground" for="confirm-password"
								>Confirm Password</label
							>
							<Input
								id="confirm-password"
								type="password"
								bind:value={model.ui.confirmPassword}
								autocomplete="new-password"
							/>
						</div>
					</div>

					<Button.Root
						type="submit"
						loading={model.isSavingPassword}
						disabled={model.isSavingPassword || model.agentBusy}
					>
						{model.isSavingPassword ? 'Saving...' : 'Update Password'}
					</Button.Root>
				</fieldset>
			</form></Card.Content
		>
	</Card.Root></Tabs.Content
>
