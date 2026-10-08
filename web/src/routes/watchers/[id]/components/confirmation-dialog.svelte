<script lang="ts">
	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Label } from '$lib/components/ui/label';
	import { Checkbox } from '$lib/components/ui/checkbox';

	interface Props {
		open: boolean;
		errorMessage?: string;
		confirmTitle: string;
		confirmDescription: string;
		confirming: boolean;
		confirmActionClass: string;
		confirmActionLabel: string;
		onConfirm: () => Promise<void>;
	}

	let {
		open = $bindable(),
		errorMessage = '',
		confirmTitle = $bindable(),
		confirmDescription = $bindable(),
		confirming = $bindable(),
		onConfirm,
		confirmActionClass,
		confirmActionLabel
	}: Props = $props();
</script>

<Dialog.Root
	bind:open
	onOpenChange={(value) => {
		if (confirming && !value) open = true;
	}}
>
	<Dialog.Content class="sm:max-w-115" showCloseButton={!confirming}>
		<Dialog.Header>
			<Dialog.Title>{confirmTitle}</Dialog.Title>
			<Dialog.Description>{confirmDescription}</Dialog.Description>
		</Dialog.Header>
		{#if errorMessage}<p role="alert" class="text-sm text-red-400">{errorMessage}</p>{/if}
		<Dialog.Footer>
			<Button.Root
				variant="outline"
				type="button"
				onclick={() => (open = false)}
				disabled={confirming}
			>
				Cancel
			</Button.Root>
			<Button.Root
				type="button"
				class={confirmActionClass}
				loading={confirming}
				onclick={onConfirm}
				disabled={confirming}
			>
				{confirming ? 'Processing...' : confirmActionLabel}
			</Button.Root>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
