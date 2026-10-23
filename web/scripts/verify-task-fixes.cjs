const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
const http = require('node:http');
const fs = require('node:fs/promises');
const path = require('node:path');

// Serve the actual production SPA with the same runtime base rewriting as
// internal/api/router.go processIndexHTML. API calls remain deterministic mocks.
async function serveBuild() {
	const root = path.resolve(__dirname, '../build');
	const index = await fs.readFile(path.join(root, 'index.html'), 'utf8');
	const types = {
		'.js': 'text/javascript',
		'.css': 'text/css',
		'.svg': 'image/svg+xml',
		'.woff2': 'font/woff2',
		'.png': 'image/png',
		'.ico': 'image/x-icon',
		'.jpg': 'image/jpeg',
		'.webp': 'image/webp',
		'.json': 'application/json'
	};
	const server = http.createServer(async (req, res) => {
		const pathname = new URL(req.url, 'http://localhost').pathname;
		const prefix = pathname.startsWith('/watcher/') ? '/watcher' : '';
		const relative = pathname.slice(prefix.length);
		try {
			const filename = path.resolve(root, '.' + relative);
			if (filename !== root && !filename.startsWith(root + path.sep))
				throw new Error('Invalid asset path');
			const stat = await fs.stat(filename).catch(() => null);
			if (stat?.isFile() && filename !== path.join(root, 'index.html')) {
				res.setHeader('Content-Type', types[path.extname(filename)] || 'application/octet-stream');
				res.end(await fs.readFile(filename));
			} else {
				if (relative.startsWith('/_app/')) throw new Error('Missing app asset');
				let html = index;
				if (prefix)
					html = html
						.replace(/base:\s*""/g, `base: "${prefix}", assets: "${prefix}"`)
						.replaceAll('href="/_app/', `href="${prefix}/_app/`)
						.replaceAll('src="/_app/', `src="${prefix}/_app/`)
						.replaceAll('import("/_app/', `import("${prefix}/_app/`)
						.replaceAll("import('/_app/", `import('${prefix}/_app/`);
				res.setHeader('Content-Type', 'text/html');
				res.end(html);
			}
		} catch {
			res.writeHead(404);
			res.end();
		}
	});
	await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
	return { server, origin: `http://127.0.0.1:${server.address().port}` };
}

(async () => {
	const { server, origin } = await serveBuild();
	const browser = await chromium.launch({
		headless: true,
		executablePath: process.env.CHROMIUM_EXECUTABLE || undefined
	});
	try {
		for (const prefix of ['', '/watcher'])
			for (const width of [1440, 390]) {
				const page = await browser.newPage({ viewport: { width, height: 1000 } });
				const errors = [];
				page.on('pageerror', (e) => errors.push(e.message));
				await page.addInitScript(() => {
					localStorage.setItem('watcher.auth.password', 'test');
					const timeout = AbortSignal.timeout.bind(AbortSignal);
					AbortSignal.timeout = (ms) => timeout(ms === 30000 ? 500 : ms);
				});
				const watcher = {
					id: 1,
					name: 'Fresh watcher',
					service_name: 'api',
					status: 'idle',
					current_version: '',
					services: [],
					metadata_url: 'https://github.com/owner/repo',
					install_dir: 'D:\\apps\\api',
					check_interval_sec: 60,
					paused: true,
					polling_activity: 'paused',
					auto_deploy: true,
					pending_catalog_id: 0,
					pending_version: '',
					max_ignored_version: '',
					last_error: '',
					last_poll_status: '',
					webhook_failure_streak: 0
				};
				const log = {
					id: 71,
					watcher_id: 1,
					version: 'v1',
					from_version: '',
					status: 'in_progress',
					logs: 'Extracting first release',
					kind: 'deployment',
					reason: 'first_release',
					triggered_by: 'agent',
					started_at: new Date().toISOString(),
					completed_at: null,
					duration_ms: 0
				};
				let failWatcher = false,
					stallWatcher = false,
					holdHistory = true,
					releaseHistory;
				let emitEvent,
					finishLogs,
					updateCalls = 0,
					completeUpdate,
					historyFailure = false;
				const historyReady = new Promise((resolve) => (releaseHistory = resolve));
				const config = {
					environment: 'production',
					github_deploy_enabled: false,
					log_dir: 'D:\\logs',
					nssm_path: 'nssm.exe',
					db_path: 'watcher.db',
					api_port: '8080',
					api_base_url: 'https://internal.invalid',
					web_assets_path: '',
					watcher_repo_url: 'https://github.com/owner/repo',
					watcher_service_name: 'Watcher',
					webhook_default_url: '',
					webhook_timeout_sec: 10,
					webhook_retry_schedule_sec: '0,10',
					webhook_auto_pause_enabled: true,
					webhook_auto_pause_after_failures: 5,
					webhook_event_retention_days: 90,
					webhook_delivery_retention_days: 30
				};
				await page.route('**/api/**', async (route) => {
					const pathname = new URL(route.request().url()).pathname.slice(prefix.length);
					const json = (body, status = 200) =>
						route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
					if (pathname === '/api/auth/status')
						return json({ authenticated: true, using_default_password: false });
					if (pathname === '/api/self/update-check')
						return json({ current_version: 'v1', latest_version: 'v2', update_available: true });
					if (pathname === '/api/self/version')
						return json({
							version: 'v1',
							go_version: 'go1.25.6',
							os: 'windows',
							arch: 'amd64',
							executable: 'watcher.exe'
						});
					if (pathname === '/api/self/config') return json(config);
					if (pathname === '/api/self/update') {
						updateCalls++;
						return new Promise((resolve) => {
							completeUpdate = async (fail) => {
								await json(
									fail
										? { error: 'Mock download failed' }
										: { message: 'update applied — restarting' },
									fail ? 500 : 202
								);
								resolve();
							};
						});
					}
					if (pathname === '/api/watchers/1') {
						if (failWatcher) return json({ error: 'Mock watcher unavailable' }, 503);
						if (stallWatcher) {
							await new Promise((r) => setTimeout(r, 1000));
							return json(watcher).catch(() => {});
						}
						return json(watcher);
					}
					if (pathname === '/api/watchers/1/events')
						return new Promise((resolve) => {
							emitEvent = async (event) => {
								await route.fulfill({
									contentType: 'text/event-stream',
									body: `data: ${JSON.stringify(event)}\n\n`.replaceAll('\\n', '\n')
								});
								resolve();
							};
						});
					if (pathname === '/api/watchers/1/deploys/71/stream')
						return new Promise((resolve) => {
							finishLogs = async () => {
								await route.fulfill({
									contentType: 'text/event-stream',
									body: 'data: DONE\n\n'.replaceAll('\\n', '\n')
								});
								resolve();
							};
						});
					if (/^\/api\/watchers\/1\/deploys\/\d+$/.test(pathname))
						return json({ ...log, id: Number(pathname.split('/').at(-1)) });
					if (pathname === '/api/watchers/1/redeploy')
						return json({
							deploy_log_id: 72,
							log_url: 'https://internal.invalid/wrong-prefix/logs/999',
							message: 'queued'
						});
					if (
						pathname === '/api/watchers/1/deploys' ||
						pathname === '/api/watchers/1/polls' ||
						pathname === '/api/watchers/1/webhook-deliveries'
					) {
						if (holdHistory) await historyReady;
						if (historyFailure) return json({ error: 'Mock history unavailable' }, 503);
						return json({ data: [], total: 0 });
					}
					if (pathname === '/api/watchers/1/versions') {
						if (holdHistory) await historyReady;
						return json({ versions: [], downloads: [] });
					}
					return json([]);
				});
				const expectText = async (text) =>
					page
						.getByText(text, { exact: false })
						.first()
						.waitFor({ state: 'visible', timeout: 5000 });
				await page.goto(`${origin}${prefix}/watchers/1`);
				await expectText('No deployment yet'); // history requests are still held
				assert.equal(await page.getByText('Loading watcher').count(), 0);
				holdHistory = false;
				releaseHistory();
				await page.getByRole('tab', { name: /^Deploys \(0\)$/ }).click();
				await expectText('No deployments yet');
				// Explicit initial failure and retry, plus bounded stalled-request recovery.
				failWatcher = true;
				await page.reload();
				await expectText('Mock watcher unavailable');
				failWatcher = false;
				await page.getByRole('button', { name: 'Try again', exact: true }).click();
				await expectText('Fresh watcher');
				stallWatcher = true;
				await page.reload();
				await expectText('Watcher details timed out');
				stallWatcher = false;
				await page.getByRole('button', { name: 'Try again', exact: true }).click();
				await expectText('Fresh watcher');
				historyFailure = true;
				await page.reload();
				await expectText('Mock history unavailable');
				historyFailure = false;
				await page.getByRole('button', { name: 'Try again', exact: true }).click();
				await page.getByRole('alert').waitFor({ state: 'hidden' });
				// The first deployment arrives while already subscribed; ignore foreign host hints.

				await expectText('No deployments yet');
				while (!emitEvent) await page.waitForTimeout(10);
				watcher.status = 'deploying';
				await emitEvent({
					type: 'deploy_started',
					watcher_id: 1,
					data: { deploy_log_id: 71, log_url: 'https://internal.invalid/logs/999' }
				});
				await page.waitForURL(`${origin}${prefix}/watchers/1/logs/71`);
				await expectText('Extracting first release');
				await expectText('in_progress');
				log.status = 'succeeded';
				log.logs += '\nFirst deployment complete';
				log.completed_at = new Date().toISOString();
				watcher.status = 'healthy';
				watcher.current_version = 'v1';
				while (!finishLogs) await page.waitForTimeout(10);
				await finishLogs();
				await expectText('First deployment complete');
				await expectText('succeeded');
				// Manual redeploy also keeps the public origin and exactly one base prefix.
				await page.goto(`${origin}${prefix}/watchers/1`);
				await expectText('Fresh watcher');
				await page.getByRole('button', { name: 'More actions' }).click();
				await page.getByRole('menuitem', { name: 'Redeploy', exact: true }).click();
				await page
					.getByRole('dialog')
					.getByRole('button', { name: 'Redeploy', exact: true })
					.click();
				await page.waitForURL(`${origin}${prefix}/watchers/1/logs/72`);
				// Banner starts update in Maintenance, retains shared operation across navigation,
				// exposes failures and explicit retry, and consumes action before refresh.
				await page.getByRole('button', { name: 'Update Watcher', exact: true }).click();
				await page.waitForURL(`${origin}${prefix}/settings?tab=maintenance`);
				await expectText('Downloading and applying the Watcher update');
				assert.equal(updateCalls, 1);
				assert.equal(
					await page.getByRole('tab', { name: 'Maintenance' }).getAttribute('aria-selected'),
					'true'
				);
				assert.equal(
					await page.getByRole('button', { name: 'Updating…', exact: true }).isDisabled(),
					true
				);
				await page.goBack();
				await page.waitForURL(`${origin}${prefix}/watchers/1/logs/72`);
				await page.goForward();
				await page.waitForURL(`${origin}${prefix}/settings?tab=maintenance`);
				await expectText('Downloading and applying the Watcher update');
				assert.equal(updateCalls, 1);
				await completeUpdate(true);
				await expectText('Mock download failed');
				await page.getByRole('button', { name: 'Try again', exact: true }).click();
				await expectText('Downloading and applying the Watcher update');
				assert.equal(updateCalls, 2);
				await completeUpdate(false);
				await expectText('update applied — restarting');
				await expectText('Reload dashboard');
				await page.reload();
				await expectText('Watcher Version');
				assert.equal(updateCalls, 2);
				await page.getByRole('tab', { name: 'Deployment', exact: true }).click();
				await page.getByLabel('Environment', { exact: true }).fill('changed');
				await page.getByRole('button', { name: 'Update Watcher', exact: true }).click();
				await expectText('Save or discard your pending changes before updating Watcher.');
				assert.equal(updateCalls, 2);
				assert.deepEqual(errors, []);
				console.log(`PASS tasks 1–3 ${prefix || '/'} ${width}px`);
				await page.close();
			}
	} finally {
		await browser.close();
		await new Promise((r) => server.close(r));
	}
})().catch((error) => {
	console.error(error);
	process.exitCode = 1;
});
