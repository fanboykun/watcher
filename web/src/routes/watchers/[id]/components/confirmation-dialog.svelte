<script lang="ts">
	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
    import { Label } from '$lib/components/ui/label';
	import { Checkbox } from '$lib/components/ui/checkbox';

    interface Props {
        open: boolean
        confirmTitle: string
        confirmDescription: string
        confirming: boolean
        confirmActionClass: string
        confirmActionLabel: string
        onConfirm: () => Promise<void>
    }

    let {
        open = $bindable(), 
        confirmTitle = $bindable(), 
        confirmDescription = $bindable(), 
        confirming = $bindable(), 
        onConfirm,
        confirmActionClass,
        confirmActionLabel,
    }: Props = $props()
</script>

<Dialog.Root bind:open={open}>
	<Dialog.Content class="sm:max-w-115">
		<Dialog.Header>
			<Dialog.Title>{confirmTitle}</Dialog.Title>
			<Dialog.Description>{confirmDescription}</Dialog.Description>
		</Dialog.Header>
		<Dialog.Footer>
			<Button.Root variant="outline" type="button" onclick={() => (open = false)} disabled={confirming}>
				Cancel
			</Button.Root>
			<Button.Root type="button" class={confirmActionClass} onclick={onConfirm} disabled={confirming}>
				{confirming ? 'Processing...' : confirmActionLabel}
			</Button.Root>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>