<script lang="ts">
	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Label } from '$lib/components/ui/label';
	import { Checkbox } from '$lib/components/ui/checkbox';

	interface Props {
		open: boolean;
		pending?: boolean;
		errorMessage?: string;
		rollbackReportGitHub: boolean;
		rollbackTargetVersion: string;
		onRollback: (version: string, reportGithub?: boolean) => Promise<void>;
	}

	let {
		open = $bindable(),
		pending = false,
		errorMessage = '',
		rollbackReportGitHub = $bindable(),
		onRollback,
		rollbackTargetVersion
	}: Props = $props();
</script>

<Dialog.Root
	bind:open
	onOpenChange={(value) => {
		if (pending && !value) open = true;
	}}
>
	<Dialog.Content class="sm:max-w-120" showCloseButton={!pending}>
		<Dialog.Header>
			<Dialog.Title>Confirm Rollback</Dialog.Title>
			<Dialog.Description>
				This will stop running services, swap the current release, and restart services.
			</Dialog.Description>
		</Dialog.Header>
		{#if errorMessage}<p role="alert" class="text-sm break-words text-red-400">
				{errorMessage}
			</p>{/if}
		<div class="space-y-3">
			<p class="text-sm">
				Target version: <span class="font-mono font-medium">{rollbackTargetVersion}</span>
			</p>
			<div class="flex items-center gap-2 py-1">
				<Checkbox
					id="rollbackReportGitHub"
					bind:checked={rollbackReportGitHub}
					disabled={pending}
				/>
				<Label for="rollbackReportGitHub" class="text-sm text-muted-foreground select-none">
					Report rollback to GitHub Deployment API
				</Label>
			</div>
		</div>
		<Dialog.Footer>
			<Button.Root
				variant="outline"
				type="button"
				disabled={pending}
				onclick={() => (open = false)}
			>
				Cancel
			</Button.Root>
			<Button.Root
				type="button"
				class="bg-amber-600 text-white hover:bg-amber-700"
				disabled={pending}
				loading={pending}
				onclick={() => onRollback(rollbackTargetVersion, rollbackReportGitHub)}
			>
				Proceed Rollback
			</Button.Root>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
