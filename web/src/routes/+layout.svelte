<script lang="ts">
	import './layout.css';
	import { page } from '$app/state';
	import { Activity, LayoutDashboard, Eye, Server, Menu, X, Clock, Settings, LogOut, AlertCircle } from '@lucide/svelte';
	import * as Button from '$lib/components/ui/button';
	import Separator from '$lib/components/ui/separator/separator.svelte';
	import { Input } from '$lib/components/ui/input';
	import { asset, resolve } from '$app/paths';
	import { onMount } from 'svelte';
	import { api, auth } from '$lib/api';

	let { children } = $props();

	let mobileOpen = $state(false);
	let checkingAuth = $state(true);
	let authenticated = $state(false);
	let loginPassword = $state('');
	let authError = $state('');
	let loggingIn = $state(false);
	let defaultPassword = $state('');

	const navItems = [
		{ href: '/', label: 'Dashboard', icon: LayoutDashboard },
		{ href: '/watchers', label: 'Watchers', icon: Eye },
		{ href: '/services', label: 'Services', icon: Server },
		{ href: '/polling', label: 'Polling', icon: Clock },
		{ href: '/logs', label: 'Logs', icon: Activity },
		{ href: '/settings', label: 'Settings', icon: Settings }
	] as const;

	onMount(() => {
		validateStoredAuth();
	});

	async function loadAuthBootstrap() {
		try {
			const bootstrap = await api.authBootstrap();
			defaultPassword = bootstrap.using_default_password ? (bootstrap.default_password ?? '') : '';
		} catch {
			defaultPassword = '';
		}
	}

	async function validateStoredAuth() {
		if (!auth.hasPassword()) {
			await loadAuthBootstrap();
			checkingAuth = false;
			return;
		}
		try {
			const status = await api.authStatus();
			authenticated = status.authenticated;
			defaultPassword = '';
		} catch {
			auth.clearPassword();
			authenticated = false;
			await loadAuthBootstrap();
		} finally {
			checkingAuth = false;
		}
	}

	async function login() {
		const password = loginPassword.trim();
		authError = '';
		if (!password) {
			authError = 'Password is required';
			return;
		}
		loggingIn = true;
		try {
			const status = await api.authLogin(password);
			auth.setPassword(password);
			authenticated = status.authenticated;
			loginPassword = '';
		} catch (e) {
			auth.clearPassword();
			authError = e instanceof Error ? e.message : 'Login failed';
		} finally {
			loggingIn = false;
		}
	}

	function logout() {
		auth.clearPassword();
		authenticated = false;
		mobileOpen = false;
	}

	function isActive(href: string) {
		if (href === '/') return page.url.pathname === '/';
		return page.url.pathname.startsWith(href);
	}
</script>

<svelte:head>
	<link rel="icon" href={asset('/watcher.ico')} sizes="any" />
	<link rel="icon" href={asset('/watcher.svg')} type="image/svg+xml" />
	<link rel="apple-touch-icon" href={asset('/watcher.png')} />
	<meta name="theme-color" content="#09090b" />
	<title>Watcher Agent</title>
</svelte:head>

{#if checkingAuth}
	<div class="dark flex min-h-screen items-center justify-center bg-background text-foreground">
		<div class="h-8 w-8 animate-spin rounded-full border-2 border-primary border-t-transparent"></div>
	</div>
{:else if !authenticated}
	<div class="dark flex min-h-screen items-center justify-center bg-background px-4 text-foreground">
		<form class="w-full max-w-sm space-y-5 rounded-lg border border-border bg-card p-6 shadow-sm" onsubmit={(e) => { e.preventDefault(); login(); }}>
			<div class="space-y-2 text-center">
				<img src={asset('/watcher.svg')} alt="Watcher" class="mx-auto h-12 w-12 rounded-lg bg-primary p-2 invert" />
				<h1 class="text-lg font-semibold">Watcher</h1>
				<p class="text-sm text-muted-foreground">Enter the dashboard password</p>
			</div>

			{#if authError}
				<div class="flex items-center gap-2 rounded-md border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-400">
					<AlertCircle class="h-4 w-4" />
					{authError}
				</div>
			{/if}

			<div class="space-y-2">
				<label class="text-sm text-muted-foreground" for="watcher-password">Password</label>
				<Input id="watcher-password" type="password" bind:value={loginPassword} autocomplete="current-password" autofocus />
				{#if defaultPassword}
					<p class="text-xs text-muted-foreground">Default password: <code class="rounded bg-muted px-1.5 py-0.5 font-mono text-foreground">{defaultPassword}</code></p>
				{/if}
			</div>

			<Button.Root type="submit" class="w-full" disabled={loggingIn}>
				{loggingIn ? 'Unlocking...' : 'Unlock Dashboard'}
			</Button.Root>
		</form>
	</div>
{:else}
	<div class="dark flex min-h-screen bg-background text-foreground">
		<!-- Sidebar -->
		<aside
			class="fixed inset-y-0 left-0 z-50 flex w-64 flex-col border-r border-border bg-card transition-transform duration-200 lg:translate-x-0 {mobileOpen
				? 'translate-x-0'
				: '-translate-x-full'}"
		>
			<div class="flex h-16 items-center gap-3 border-b border-border px-6">
				<img src={asset('/watcher.svg')} alt="Watcher" class="h-8 w-8 rounded-lg bg-primary p-1.5 invert" />
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
			<div class="space-y-3 p-4">
				<Button.Root variant="outline" class="w-full justify-start" onclick={logout}>
					<LogOut class="mr-2 h-4 w-4" />
					Logout
				</Button.Root>
				<a
					href="https://github.com/fanboykun/watcher"
					target="_blank"
					rel="noopener noreferrer"
					class="inline-flex w-full items-center justify-center gap-1.5 text-[11px] text-muted-foreground transition-colors hover:text-foreground"
				>
					<svg class="h-3.5 w-3.5" fill="currentColor" viewBox="0 0 24 24"><path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/></svg>
					Watcher Agent
				</a>
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
{/if}
