<script lang="ts">
	import './layout.css';
	import { Eye, ShieldCheck, ArrowRight, AlertCircle } from '@lucide/svelte';
	import * as Button from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { asset } from '$app/paths';
	import AppShell from '$lib/components/shell/app-shell.svelte';
	import UpdateNotice from '$lib/components/shell/update-notice.svelte';
	import { onMount } from 'svelte';
	import { api, auth, type SelfUpdateCheckResponse } from '$lib/api';
	import {
		dismissSelfUpdate,
		getSelfUpdateSnapshot,
		lookupSelfUpdate,
		subscribeSelfUpdate
	} from '$lib/self-update';

	let { children } = $props();

	let checkingAuth = $state(true);
	let authenticated = $state(false);
	let loginPassword = $state('');
	let authError = $state('');
	let loggingIn = $state(false);
	let defaultPassword = $state('');
	let selfUpdateInfo = $state<SelfUpdateCheckResponse | null>(null);
	let showSelfUpdateBanner = $state(false);
	let selfUpdateBusy = $state(false);

	function syncSelfUpdateState() {
		const snapshot = getSelfUpdateSnapshot();
		selfUpdateInfo = snapshot.info;
		showSelfUpdateBanner = snapshot.shouldNotify;
		selfUpdateBusy = snapshot.updateStatus === 'updating' || snapshot.updateStatus === 'applied';
	}

	onMount(() => {
		syncSelfUpdateState();
		const unsubscribe = subscribeSelfUpdate((snapshot) => {
			selfUpdateInfo = snapshot.info;
			showSelfUpdateBanner = snapshot.shouldNotify;
			selfUpdateBusy = snapshot.updateStatus === 'updating' || snapshot.updateStatus === 'applied';
		});
		validateStoredAuth();
		return unsubscribe;
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
			if (status.authenticated) {
				void lookupSelfUpdate({ silent: true });
			}
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
			if (status.authenticated) {
				void lookupSelfUpdate({ silent: true });
			}
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
	}

	function dismissUpdateBanner() {
		if (!selfUpdateInfo?.latest_version) return;
		dismissSelfUpdate(selfUpdateInfo.latest_version);
	}
</script>

<svelte:head>
	<link rel="icon" href={asset('/watcher.ico')} sizes="any" />
	<link rel="icon" href={asset('/watcher.svg')} type="image/svg+xml" />
	<link rel="apple-touch-icon" href={asset('/watcher.png')} />
	<meta name="theme-color" content="#111315" />
	<title>Watcher · Deployment control</title>
</svelte:head>

{#if checkingAuth}
	<div class="dark flex min-h-screen items-center justify-center bg-background text-foreground">
		<div
			class="h-8 w-8 animate-spin rounded-full border-2 border-primary border-t-transparent"
		></div>
	</div>
{:else if !authenticated}
	<div class="login-screen">
		<form
			class="login-panel"
			onsubmit={(e) => {
				e.preventDefault();
				login();
			}}
		>
			<div class="mb-8">
				<span class="brand-mark mb-8"><Eye class="size-7" /></span>
				<p class="eyebrow mb-3">Deployment control</p>
				<h1 class="text-3xl font-semibold tracking-tight">
					Welcome to watcher<span class="text-primary">.</span>
				</h1>
				<p class="mt-3 text-sm leading-relaxed text-muted-foreground">
					Your releases. Your infrastructure.<br />One place to keep it all running.
				</p>
			</div>

			{#if authError}
				<div
					class="flex items-center gap-2 rounded-md border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-400"
				>
					<AlertCircle class="h-4 w-4" />
					{authError}
				</div>
			{/if}

			<div class="space-y-2">
				<label class="text-sm text-muted-foreground" for="watcher-password">Password</label>
				<Input
					id="watcher-password"
					type="password"
					bind:value={loginPassword}
					autocomplete="current-password"
				/>
				{#if defaultPassword}
					<p class="text-xs text-muted-foreground">
						Default password: <code class="rounded bg-muted px-1.5 py-0.5 font-mono text-foreground"
							>{defaultPassword}</code
						>
					</p>
				{/if}
			</div>

			<Button.Root type="submit" class="w-full" disabled={loggingIn}>
				{loggingIn ? 'Signing in…' : 'Open workspace'}
				<ArrowRight class="size-4" />
			</Button.Root>
			<p class="flex items-center justify-center gap-2 text-xs text-muted-foreground">
				<ShieldCheck class="size-3.5" /> Authorized operators only
			</p>
		</form>
	</div>
{:else}
	<AppShell onlogout={logout} updateAvailable={showSelfUpdateBanner}>
		{#if showSelfUpdateBanner && selfUpdateInfo?.update_available}
			<UpdateNotice info={selfUpdateInfo} busy={selfUpdateBusy} ondismiss={dismissUpdateBanner} />
		{/if}
		{@render children()}
	</AppShell>
{/if}
