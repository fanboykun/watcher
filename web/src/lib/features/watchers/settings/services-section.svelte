<script lang="ts">
	import * as Tabs from '$lib/components/ui/tabs';
	import { resolve } from '$app/paths';
	import { type Service } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import ServicesTab from '$lib/features/watchers/components/services-tab.svelte';
	import type { createWatcherSettings } from '$lib/features/watchers/settings/settings-controller.svelte';
	let { model }: { model: ReturnType<typeof createWatcherSettings> } = $props();
</script>

{#if model.watcher}
	<Tabs.Content value="services"
		><Card.Root class="border-border bg-card" id="services">
			<Card.Header>
				<Card.Title>Service Settings</Card.Title>
				<Card.Description>
					Add, edit, or remove the managed services tied to this watcher.
				</Card.Description>
			</Card.Header>
			<Card.Content>
				<ServicesTab
					watcher={model.watcher}
					createHref={resolve(`/watchers/${model.id}/services/new`)}
					editHrefBase="/services"
					onDeleteService={model.deleteService}
				/>
			</Card.Content>
		</Card.Root></Tabs.Content
	>
{/if}
