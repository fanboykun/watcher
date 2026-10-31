<script lang="ts">
	import { Eye, ShieldCheck, ArrowRight, AlertCircle, LogOut } from '@lucide/svelte';
	import * as Button from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { asset } from '$app/paths';
	import AppShell from '$lib/components/shell/app-shell.svelte';
	import UpdateNotice from '$lib/components/shell/update-notice.svelte';
	import ErrorScreen from '$lib/components/errors/error-screen.svelte';
	import { isUnauthorized } from '$lib/components/errors/error-model';
	import { createQuery, createMutation, useQueryClient } from '@tanstack/svelte-query';
	import { api, auth } from '$lib/api';
	import { queries } from '$lib/query/options';
	import { createSelfUpdate } from '$lib/query/self-update.svelte';
	let { children } = $props();
	const client = useQueryClient();
	let credentials = $state(auth.hasPassword());
	let loginPassword = $state('');
	let validationError = $state('');
	const session = createQuery(() => ({ ...queries.auth(), enabled: credentials, gcTime: 0 }));
	// Only a 401 ends the session. Outages, timeouts and restarts keep the stored
	// credential and the mounted workspace, including any unsaved drafts.
	const authenticated = $derived(credentials && session.data?.authenticated === true);
	const checkingAuth = $derived(credentials && session.isPending);
	const unreachable = $derived(
		credentials && !session.data && session.isError && !isUnauthorized(session.error)
	);
	const bootstrap = createQuery(() => ({ ...queries.bootstrap(), enabled: !credentials }));
	const defaultPassword = $derived(
		bootstrap.data?.using_default_password ? (bootstrap.data.default_password ?? '') : ''
	);
	const loginMutation = createMutation(() => ({ mutationFn: api.authLogin }));
	const loggingIn = $derived(loginMutation.isPending);
	const authError = $derived(validationError || loginMutation.error?.message || '');
	const update = createSelfUpdate(() => authenticated);
	const selfUpdateInfo = $derived(update.check.data ?? null);
	const showSelfUpdateBanner = $derived(update.shouldNotify);
	const selfUpdateBusy = $derived(update.busy);
	async function login() {
		validationError = '';
		const password = loginPassword.trim();
		if (!password) {
			validationError = 'Password is required';
			return;
		}
		try {
			const status = await loginMutation.mutateAsync(password);
			auth.setPassword(password);
			client.setQueryData(queries.auth().queryKey, status);
			credentials = true;
			loginPassword = '';
		} catch {
			auth.clearPassword();
		}
	}
	async function logout() {
		credentials = false;
		auth.clearPassword();
		await client.cancelQueries();
		client.clear();
		loginMutation.reset();
	}
	// Any read Watcher answers with 401 means the stored credential stopped working.
	$effect(() =>
		client.getQueryCache().subscribe((event) => {
			if (
				event.type === 'updated' &&
				event.action.type === 'error' &&
				isUnauthorized(event.action.error) &&
				credentials
			)
				queueMicrotask(() => void logout());
		})
	);
	function dismissUpdateBanner() {
		if (selfUpdateInfo?.latest_version) update.dismiss(selfUpdateInfo.latest_version);
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
{:else if unreachable}
	<ErrorScreen error={session.error} scope="app" onRetry={() => session.refetch()}>
		{#snippet actions()}
			<Button.Root variant="ghost" onclick={logout}><LogOut class="size-4" />Sign out</Button.Root>
		{/snippet}
	</ErrorScreen>
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
