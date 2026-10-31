<script lang="ts">
	import { beforeNavigate, goto } from '$app/navigation';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Button from '$lib/components/ui/button';
	let { dirty, label = 'configuration' }: { dirty: boolean; label?: string } = $props();
	let destination = $state<URL | null>(null);
	let delta: number | undefined;
	let bypass = false;
	beforeNavigate((navigation) => {
		if (bypass) {
			bypass = false;
			return;
		}
		if (
			!dirty ||
			navigation.willUnload ||
			navigation.to?.url.pathname === navigation.from?.url.pathname
		)
			return;
		navigation.cancel();
		delta = navigation.type === 'popstate' ? navigation.delta : undefined;
		destination = navigation.to?.url ?? null;
	});
	function unload(event: BeforeUnloadEvent) {
		if (!dirty) return;
		event.preventDefault();
		event.returnValue = '';
	}
	async function leave() {
		const target = destination;
		if (!target) return;
		destination = null;
		bypass = true;
		if (delta !== undefined) {
			history.go(delta);
			return;
		}
		// The URL comes from SvelteKit's already-resolved navigation destination.
		// eslint-disable-next-line svelte/no-navigation-without-resolve
		await goto(target);
	}
</script>

<svelte:window onbeforeunload={unload} />
<Dialog.Root
	open={destination !== null}
	onOpenChange={(open) => {
		if (!open) destination = null;
	}}
>
	<Dialog.Content>
		<Dialog.Header
			><Dialog.Title>Leave unsaved {label}?</Dialog.Title><Dialog.Description
				>Your edits have not been saved. Stay here to continue editing, or leave and discard them.</Dialog.Description
			></Dialog.Header
		>
		<Dialog.Footer
			><Button.Root variant="outline" onclick={() => (destination = null)}>Keep editing</Button.Root
			><Button.Root variant="destructive" onclick={leave}>Discard and leave</Button.Root
			></Dialog.Footer
		>
	</Dialog.Content>
</Dialog.Root>
