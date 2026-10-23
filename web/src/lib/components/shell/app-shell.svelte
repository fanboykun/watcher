<script lang="ts">
	import { page } from '$app/state';
	import { base, resolve } from '$app/paths';
	import type { Snippet } from 'svelte';
	import {
		Activity,
		LayoutDashboard,
		Eye,
		Server,
		Menu,
		Clock,
		Settings,
		LogOut,
		Webhook,
		ArrowUpRight,
		Radio,
		ChevronRight
	} from '@lucide/svelte';
	import * as Sheet from '$lib/components/ui/sheet';
	import * as Button from '$lib/components/ui/button';
	let {
		children,
		onlogout,
		updateAvailable = false
	}: { children: Snippet; onlogout: () => void; updateAvailable?: boolean } = $props();
	let mobileOpen = $state(false);
	const groups = [
		{
			label: 'Workspace',
			items: [
				{ href: '/', label: 'Overview', icon: LayoutDashboard },
				{ href: '/watchers', label: 'Watchers', icon: Eye },
				{ href: '/services', label: 'Services', icon: Server }
			]
		},
		{
			label: 'Observability',
			items: [
				{ href: '/polling', label: 'Polling', icon: Radio },
				{ href: '/logs', label: 'Agent logs', icon: Activity },
				{ href: '/webhooks', label: 'Webhooks', icon: Webhook }
			]
		},
		{ label: 'Administration', items: [{ href: '/settings', label: 'Settings', icon: Settings }] }
	] as const;
	function isActive(href: (typeof groups)[number]['items'][number]['href']) {
		const target = resolve(href);
		return href === '/'
			? page.url.pathname === base || page.url.pathname === `${base}/`
			: page.url.pathname === target || page.url.pathname.startsWith(`${target}/`);
	}
	const current = $derived(
		groups.flatMap((group) => [...group.items]).find((item) => isActive(item.href))?.label ||
			'Workspace'
	);
</script>

{#snippet navigation()}
	<a class="brand" href={resolve('/')} onclick={() => (mobileOpen = false)}>
		<span class="brand-mark"><Eye class="size-6" strokeWidth={1.6} /></span>
		<span
			><strong>watcher<span class="text-primary">.</span></strong><span class="brand-caption"
				>DEPLOYMENT CONTROL</span
			></span
		>
	</a>
	<nav class="console-navigation" aria-label="Main navigation">
		{#each groups as group (group.label)}
			<div class="nav-group">
				<p class="eyebrow">{group.label}</p>
				{#each group.items as item (item.href)}
					<a
						href={resolve(item.href)}
						aria-current={isActive(item.href) ? 'page' : undefined}
						class={['nav-item', isActive(item.href) && 'nav-item-active']}
						onclick={() => (mobileOpen = false)}
					>
						<item.icon class="size-[18px]" strokeWidth={1.7} /><span>{item.label}</span>
						{#if item.href === '/settings' && updateAvailable}<span
								class="ml-auto size-1.5 rounded-full bg-primary"
								aria-label="Update available"
							></span>{/if}
					</a>
				{/each}
			</div>
		{/each}
	</nav>
	<div class="sidebar-bottom">
		<div class="agent-plaque">
			<span class="agent-glyph"><Clock class="size-4" /></span>
			<div>
				<p class="text-xs font-medium">Always on watch.</p>
				<p class="mt-1 text-[11px] text-muted-foreground">Pull. Deploy. Keep moving.</p>
			</div>
		</div>
		<div class="mt-4 flex items-center justify-between gap-2">
			<a
				href="https://github.com/fanboykun/watcher"
				target="_blank"
				rel="noreferrer"
				class="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
				>Project on GitHub <ArrowUpRight class="size-3" /></a
			>
			<button class="signout" onclick={onlogout} aria-label="Sign out" title="Sign out"
				><LogOut class="size-4" /></button
			>
		</div>
	</div>
{/snippet}

<div class="console-shell">
	<a class="skip-link" href="#main-content">Skip to content</a>
	<aside class="console-sidebar hidden lg:flex">{@render navigation()}</aside>
	<Sheet.Root bind:open={mobileOpen}>
		<Sheet.Content side="left" class="console-sidebar w-[260px] gap-0 p-0" showCloseButton={true}>
			<Sheet.Title class="sr-only">Watcher navigation</Sheet.Title>
			<Sheet.Description class="sr-only">Navigate your deployment workspace.</Sheet.Description>
			{@render navigation()}
		</Sheet.Content>
	</Sheet.Root>
	<div class="console-workspace">
		<header class="console-topbar">
			<Button.Root
				variant="ghost"
				size="icon"
				class="lg:hidden"
				aria-label="Open navigation"
				onclick={() => (mobileOpen = true)}><Menu class="size-5" /></Button.Root
			>
			<div class="flex items-center gap-2.5 text-xs">
				<span class="text-muted-foreground">Workspace</span><ChevronRight
					class="size-3 text-muted-foreground"
				/><span>{current}</span>
			</div>
			<div class="ml-auto flex items-center gap-3">
				<span class="topbar-tag">WINDOWS DEPLOYMENT AGENT</span><span
					class="console-avatar"
					title="Watcher dashboard">W</span
				>
			</div>
		</header>
		<main id="main-content" class="console-content" tabindex="-1">{@render children()}</main>
		<footer class="console-footer">
			<span>WATCHER <span class="mx-2 opacity-40">/</span> DEPLOYMENT CONTROL</span><span
				>Built to keep things running.</span
			>
		</footer>
	</div>
</div>
