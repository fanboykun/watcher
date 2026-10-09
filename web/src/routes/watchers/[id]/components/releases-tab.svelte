<script lang="ts">
	import { onMount } from 'svelte';
	import {
		api,
		type Watcher,
		type ReleaseCatalogResponse,
		type CatalogRelease,
		type CatalogDownload
	} from '$lib/api';
	import * as Button from '$lib/components/ui/button';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Badge } from '$lib/components/ui/badge';
	import {
		Archive,
		ChevronDown,
		Download,
		ExternalLink,
		RefreshCw,
		SlidersHorizontal,
		Rocket
	} from '@lucide/svelte';
	import { filesize } from 'filesize';
	import { formatDate } from '$lib/utils';

	let {
		watcher,
		onNavigate
	}: { watcher: Watcher; onNavigate: (tab: 'candidates' | 'deploys') => Promise<void> } = $props();
	let catalog = $state.raw<ReleaseCatalogResponse | null>(null);
	let loading = $state(true);
	let error = $state('');
	let message = $state('');
	let busyRelease = $state<number | null>(null);
	let selectedAssets = $state<Record<number, string>>({});
	let deployTarget = $state<CatalogDownload | null>(null);
	let confirmOpen = $state(false);
	let controller: AbortController | undefined;
	let mounted = false;

	function zipAssets(release: CatalogRelease) {
		return release.assets.filter((asset) => asset.name.toLowerCase().endsWith('.zip'));
	}
	function selectedAsset(release: CatalogRelease) {
		const assets = zipAssets(release);
		return (
			assets.find((asset) => String(asset.id) === selectedAssets[release.id]) ??
			assets.find((asset) => asset.name.startsWith(watcher.service_name)) ??
			assets[0]
		);
	}
	function downloadFor(release: CatalogRelease) {
		return catalog?.downloads.find(
			(download) =>
				download.release_id === release.id && download.asset_id === selectedAsset(release)?.id
		);
	}
	async function load(page = 1) {
		controller?.abort();
		const requestController = new AbortController();
		controller = requestController;
		loading = true;
		error = '';
		try {
			const result = await api.getReleaseCatalog(watcher.id, page, requestController.signal);
			if (mounted && !requestController.signal.aborted) catalog = result;
		} catch (cause) {
			if (mounted && !requestController.signal.aborted)
				error = cause instanceof Error ? cause.message : 'Could not load releases.';
		} finally {
			if (mounted && !requestController.signal.aborted) loading = false;
		}
	}
	async function download(release: CatalogRelease) {
		const asset = selectedAsset(release);
		if (!asset || busyRelease !== null) return;
		busyRelease = release.id;
		error = '';
		message = '';
		try {
			const result = await api.downloadCatalogRelease(watcher.id, release.id, asset.id);
			if (mounted && catalog) {
				catalog = { ...catalog, downloads: [result, ...catalog.downloads] };
				message = `${result.asset_name} downloaded to Watcher. Ready to configure.`;
			}
		} catch (cause) {
			if (mounted) error = cause instanceof Error ? cause.message : 'Download failed.';
		} finally {
			if (mounted) busyRelease = null;
		}
	}
	async function selectCandidate(release: CatalogRelease) {
		const download = downloadFor(release);
		if (!download || busyRelease !== null) return;
		busyRelease = release.id;
		error = '';
		try {
			await api.selectCatalogCandidate(watcher.id, download.id);
			if (mounted) await onNavigate('candidates');
		} catch (cause) {
			if (mounted) error = cause instanceof Error ? cause.message : 'Could not select candidate.';
		} finally {
			if (mounted) busyRelease = null;
		}
	}
	async function deploy() {
		if (!deployTarget || busyRelease !== null) return;
		busyRelease = deployTarget.release_id;
		error = '';
		try {
			await api.deployCatalogRelease(watcher.id, deployTarget.id);
			confirmOpen = false;
			if (mounted) await onNavigate('deploys');
		} catch (cause) {
			if (mounted) {
				confirmOpen = false;
				error = cause instanceof Error ? cause.message : 'Could not queue deployment.';
			}
		} finally {
			if (mounted) busyRelease = null;
		}
	}
	onMount(() => {
		mounted = true;
		void load();
		return () => {
			mounted = false;
			controller?.abort();
		};
	});
</script>

<section
	class="overflow-hidden rounded-xl border border-border bg-card"
	aria-label="GitHub releases catalog"
>
	<header class="section-toolbar border-b border-border px-5 py-5">
		<div class="space-y-1">
			<div class="flex items-center gap-2">
				<Archive class="size-4 text-muted-foreground" />
				<h2 class="font-semibold">GitHub releases</h2>
			</div>
			<p class="text-sm text-muted-foreground">
				{catalog?.repository ?? 'Repository catalog'} · Download, configure, then deploy.
			</p>
		</div>
		<Button.Root
			variant="outline"
			size="sm"
			disabled={loading || busyRelease !== null}
			onclick={() => load(catalog?.page ?? 1)}><RefreshCw />Refresh</Button.Root
		>
	</header>
	<div
		class="border-b border-border bg-muted/20 px-5 py-3 text-xs leading-relaxed text-muted-foreground"
	>
		Downloads are staged on Watcher’s host. Selecting a candidate holds automatic deployment while
		you review its configuration. Deployment uses the exact downloaded ZIP.
	</div>
	{#if error}<p
			role="alert"
			class="m-5 rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
		>
			{error}
		</p>{/if}
	{#if message}<p role="status" class="mx-5 mt-4 text-sm text-emerald-500">{message}</p>{/if}
	{#if loading && !catalog}
		<p role="status" class="px-5 py-12 text-sm text-muted-foreground">Loading GitHub releases…</p>
	{:else if catalog}
		<div class="divide-y divide-border" aria-busy={loading}>
			{#each catalog.releases as release (release.id)}
				{@const asset = selectedAsset(release)}
				{@const downloaded = downloadFor(release)}
				<article class="space-y-3 px-5 py-5" aria-label={`Release ${release.tag_name}`}>
					<div class="section-toolbar">
						<div class="min-w-0 space-y-1.5">
							<div class="flex flex-wrap items-center gap-2">
								<h3 class="font-medium break-all">{release.name || release.tag_name}</h3>
								{#if release.prerelease}<Badge variant="outline">Prerelease</Badge>{/if}
								{#if release.draft}<Badge variant="outline">Draft</Badge>{/if}
								{#if downloaded?.version === watcher.current_version || release.tag_name === watcher.current_version}<Badge
										variant="secondary">Current</Badge
									>{/if}
								{#if downloaded && downloaded.id === watcher.pending_catalog_id}<Badge
										variant="secondary">Selected candidate</Badge
									>{/if}
							</div>
							<p class="text-xs text-muted-foreground">
								<span class="font-mono">{release.tag_name}</span> · {release.published_at
									? formatDate(release.published_at)
									: 'Unpublished'}
							</p>
						</div>
						<DropdownMenu.Root>
							<DropdownMenu.Trigger
								class="inline-flex h-8 shrink-0 items-center gap-2 rounded-md border border-border px-3 text-sm hover:bg-muted disabled:opacity-50"
								disabled={busyRelease !== null || loading}
								aria-label={`Actions for ${release.tag_name}`}
								>Actions<ChevronDown class="size-3.5" /></DropdownMenu.Trigger
							>
							<DropdownMenu.Content align="end">
								<DropdownMenu.Item
									disabled={!asset || release.draft}
									onclick={() => download(release)}
									><Download />{downloaded
										? 'Download again'
										: 'Download artifact'}</DropdownMenu.Item
								>
								<DropdownMenu.Item
									disabled={!downloaded || release.draft}
									onclick={() => selectCandidate(release)}
									><SlidersHorizontal />Configure candidate</DropdownMenu.Item
								>
								<DropdownMenu.Item
									disabled={!downloaded || release.draft}
									onclick={() => {
										deployTarget = downloaded ?? null;
										confirmOpen = true;
									}}><Rocket />Deploy release…</DropdownMenu.Item
								>

								<DropdownMenu.Item
									onclick={() => window.open(release.html_url, '_blank', 'noopener,noreferrer')}
									><ExternalLink />View on GitHub</DropdownMenu.Item
								>
							</DropdownMenu.Content>
						</DropdownMenu.Root>
					</div>
					<div class="flex flex-wrap items-center gap-x-4 gap-y-2">
						{#if asset}
							<label class="flex min-w-0 items-center gap-2 text-xs text-muted-foreground"
								>Artifact
								<select
									class="max-w-full rounded-md border border-border bg-background px-2 py-1.5 font-mono text-xs text-foreground sm:max-w-96"
									aria-label={`Artifact for ${release.tag_name}`}
									disabled={busyRelease !== null}
									value={String(asset.id)}
									onchange={(event) => {
										selectedAssets[release.id] = event.currentTarget.value;
									}}
								>
									{#each zipAssets(release) as option (option.id)}<option value={String(option.id)}
											>{option.name}</option
										>{/each}
								</select>
							</label>
							<span class="text-xs text-muted-foreground">{filesize(asset.size)}</span>
							{#if busyRelease === release.id}<span role="status" class="text-xs text-amber-500"
									>Working…</span
								>
							{:else if downloaded}<span class="text-xs text-emerald-500"
									>Downloaded · {downloaded.version}</span
								>
							{:else}<span class="text-xs text-muted-foreground">Not downloaded</span>{/if}
						{:else}<span class="text-xs text-muted-foreground">No deployable ZIP assets</span>{/if}
					</div>
					{#if release.body}<details class="text-sm">
							<summary class="cursor-pointer text-xs text-muted-foreground hover:text-foreground"
								>Release notes</summary
							>
							<pre
								class="mt-3 max-h-80 overflow-auto rounded-md bg-muted/30 p-3 font-sans text-sm leading-relaxed whitespace-pre-wrap">{release.body}</pre>
						</details>{/if}
				</article>
			{:else}<p class="px-5 py-12 text-sm text-muted-foreground">
					No releases on this page.
				</p>{/each}
		</div>
		<footer class="flex items-center justify-between border-t border-border px-5 py-3">
			<span class="text-xs text-muted-foreground"
				>Page {catalog.page}{loading ? ' · Loading…' : ''}</span
			>
			<div class="flex gap-2">
				<Button.Root
					variant="outline"
					size="sm"
					disabled={loading || catalog.page <= 1 || busyRelease !== null}
					onclick={() => load((catalog?.page ?? 1) - 1)}>Previous</Button.Root
				><Button.Root
					variant="outline"
					size="sm"
					disabled={loading || !catalog.has_next || busyRelease !== null}
					onclick={() => load((catalog?.page ?? 1) + 1)}>Next</Button.Root
				>
			</div>
		</footer>
	{/if}
</section>

<Dialog.Root bind:open={confirmOpen}>
	<Dialog.Content>
		<Dialog.Header
			><Dialog.Title>Deploy {deployTarget?.version}?</Dialog.Title><Dialog.Description
				>The service will stop while Watcher activates this release. Saved candidate configuration
				is applied before health checks. Failed activation uses the existing rollback flow.</Dialog.Description
			></Dialog.Header
		>
		<div class="space-y-2 rounded-lg border border-border bg-muted/20 p-3 text-sm">
			<p class="font-mono break-all">{deployTarget?.asset_name}</p>
			<p class="text-muted-foreground">
				{watcher.current_version || 'Not deployed'} → {deployTarget?.version}
			</p>
			<p class="text-xs text-muted-foreground">
				Automatic polling keeps its configured release reference after this manual deployment.
			</p>
		</div>
		<Dialog.Footer
			><Button.Root
				variant="outline"
				disabled={busyRelease !== null}
				onclick={() => {
					confirmOpen = false;
				}}>Cancel</Button.Root
			><Button.Root loading={busyRelease !== null} onclick={deploy}
				><Rocket />Deploy release</Button.Root
			></Dialog.Footer
		>
	</Dialog.Content>
</Dialog.Root>
