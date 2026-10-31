<script lang="ts">
	import * as Tabs from '$lib/components/ui/tabs';
	import { type Watcher } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import type { createWatcherSettings } from '$lib/features/watchers/settings/settings-controller.svelte';
	import SourceTypePicker from '$lib/features/watchers/source-type-picker.svelte';
	import FeedAuthFields from '$lib/features/watchers/feed-auth-fields.svelte';
	import {
		isGitHubSource,
		sourceLocationLabel,
		takesFeedCredentials
	} from '$lib/features/watchers/presentation';
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
					<Label>Release source</Label>
					<SourceTypePicker bind:value={model.ui.editSourceType} idPrefix="editSource" />
				</div>
				<div class="space-y-2">
					<Label for="editMetadataURL">{sourceLocationLabel(model.ui.editSourceType)}</Label>
					<Input
						id="editMetadataURL"
						bind:value={model.ui.editMetadataURL}
						aria-describedby="editMetadataURL-help"
					/>
					<p id="editMetadataURL-help" class="text-xs text-muted-foreground">
						{#if !isGitHubSource(model.ui.editSourceType)}
							Feed root that holds <code>version.json</code>.
						{:else}
							GitHub repository URL or release metadata URL that Watcher checks for new versions.
						{/if}
					</p>
				</div>
				{#if takesFeedCredentials(model.ui.editSourceType)}
					<FeedAuthFields
						bind:auth={model.ui.editFeedAuth}
						idPrefix="editFeedAuth"
						s3={model.ui.editSourceType === 's3'}
						savedSecretMasked={model.watcher.has_feed_auth_secret
							? model.watcher.feed_auth_secret_masked || 'set'
							: ''}
					/>
				{/if}
				<div class="space-y-2">
					<Label for="editReleaseRef">Release Ref</Label>
					<Input
						id="editReleaseRef"
						bind:value={model.ui.editReleaseRef}
						placeholder="latest or v1.2.3"
					/>
					<p class="text-xs text-muted-foreground">
						{#if !isGitHubSource(model.ui.editSourceType)}
							Use <code>latest</code> to follow the feed, or pin a tag published under
							<code>releases/</code>.
						{:else}
							Use <code>latest</code> to follow new releases, or pin this watcher to a specific release
							tag.
						{/if}
					</p>
				</div></Card.Content
			></Card.Root
		></Tabs.Content
	>
{/if}
