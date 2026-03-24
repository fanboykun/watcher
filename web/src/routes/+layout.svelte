<script lang="ts">
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';
	import { page } from '$app/state';
	import { Activity, LayoutDashboard, Eye, Server, Menu, X, Clock, Settings } from '@lucide/svelte';
	import * as Button from '$lib/components/ui/button';
	import Separator from '$lib/components/ui/separator/separator.svelte';
	import { resolve } from '$app/paths';

	let { children } = $props();

	let mobileOpen = $state(false);

	const navItems = [
		{ href: '/', label: 'Dashboard', icon: LayoutDashboard },
		{ href: '/watchers', label: 'Watchers', icon: Eye },
		{ href: '/services', label: 'Services', icon: Server },
		{ href: '/polling', label: 'Polling', icon: Clock },
		{ href: '/logs', label: 'Logs', icon: Activity },
		{ href: '/settings', label: 'Settings', icon: Settings }
	] as const;

	function isActive(href: string) {
		if (href === '/') return page.url.pathname === '/';
		return page.url.pathname.startsWith(href);
	}
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>Watcher Agent</title>
</svelte:head>

<div class="dark flex min-h-screen bg-background text-foreground">
	<!-- Sidebar -->
	<aside
		class="fixed inset-y-0 left-0 z-50 flex w-64 flex-col border-r border-border bg-card transition-transform duration-200 lg:translate-x-0 {mobileOpen
			? 'translate-x-0'
			: '-translate-x-full'}"
	>
		<div class="flex h-16 items-center gap-3 border-b border-border px-6">
			<div
				class="flex h-8 w-8 items-center justify-center rounded-lg bg-primary text-primary-foreground"
			>
				<Eye class="h-4 w-4" />
			</div>
			<div>
				<h1 class="text-sm font-semibold">Watcher</h1>
				<p class="text-[11px] text-muted-foreground">Deploy Agent</p>
			</div>
		</div>

		<nav class="flex-1 space-y-1 p-3">
			{#each navItems as item (item.href)}
				<a
					href={resolve(item.href)}
					onclick={() => (mobileOpen = false)}
					class="flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors {isActive(
						item.href
					)
						? 'bg-accent text-accent-foreground'
						: 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'}"
				>
					<item.icon class="h-4 w-4" />
					{item.label}
				</a>
			{/each}
		</nav>

		<Separator />
		<div class="p-4">
			<p class="text-[11px] text-muted-foreground">Watcher Agent</p>
		</div>
	</aside>

	<!-- Mobile toggle -->
	<Button.Root
		variant="ghost"
		size="icon"
		class="fixed top-4 left-4 z-50 lg:hidden"
		onclick={() => (mobileOpen = !mobileOpen)}
	>
		{#if mobileOpen}
			<X class="h-5 w-5" />
		{:else}
			<Menu class="h-5 w-5" />
		{/if}
	</Button.Root>

	<!-- Mobile overlay -->
	{#if mobileOpen}
		<button
			class="fixed inset-0 z-40 bg-black/50 lg:hidden"
			onclick={() => (mobileOpen = false)}
			aria-label="Close menu"
		></button>
	{/if}

	<!-- Main content -->
	<main class="flex-1 lg:ml-64">
		<div class="mx-auto max-w-6xl p-4 lg:p-6">
			{@render children()}
		</div>
	</main>
</div>
