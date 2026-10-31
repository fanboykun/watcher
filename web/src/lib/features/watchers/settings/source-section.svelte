<script lang="ts">
	import * as Tabs from '$lib/components/ui/tabs';
	import { type Watcher } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import type { createWatcherSettings } from '$lib/features/watchers/settings/settings-controller.svelte';
	let { model }: { model: ReturnType<typeof createWatcherSettings> } = $props();
</script>

{#if model.watcher}
	<Tabs.Content value="source"
		><Card.Root
			><Card.Header
				><Card.Title>Release source</Card.Title><Card.Description
					>Choose the repository or metadata feed and release to follow.</Card.Description
				></Card.Header
			><Card.Content class="space-y-4"
				><div class="space-y-2">
					<Label for="editMetadataURL">Metadata URL</Label>
					<Input
						id="editMetadataURL"
						bind:value={model.ui.editMetadataURL}
						aria-describedby="editMetadataURL-help"
					/>
					<p id="editMetadataURL-help" class="text-xs text-muted-foreground">
						GitHub repository URL or release metadata URL that Watcher checks for new versions.
					</p>
				</div>
				<div class="space-y-2">
					<Label for="editReleaseRef">Release Ref</Label>
					<Input
						id="editReleaseRef"
						bind:value={model.ui.editReleaseRef}
						placeholder="latest or v1.2.3"
					/>
					<p class="text-xs text-muted-foreground">
						Use <code>latest</code> to follow new releases, or pin this watcher to a specific release
						tag.
					</p>
				</div></Card.Content
			></Card.Root
		></Tabs.Content
	>
{/if}
