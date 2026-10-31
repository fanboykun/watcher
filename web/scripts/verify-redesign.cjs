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
				const page = await browser.newPage({ viewport: { width, height: 1050 } });
				page.setDefaultTimeout(8000);
				const errors = [];
				page.on('pageerror', (error) => {
					errors.push(error.message);
					console.error('Browser error:', error.message);
				});
				await page.addInitScript(() => localStorage.setItem('watcher.auth.password', 'fixture'));
				let empty = false,
					failed = false,
					pauses = 0,
					serviceActions = 0;
				const service = {
					id: 1,
					watcher_id: 1,
					watcher_name: 'Commerce API',
					service_type: 'nssm',
					windows_service_name: 'commerce-api',
					binary_name: 'api.exe',
					health_check_url: 'http://localhost:9000/health',
					last_service_status: 'running',
					last_health_status: 'healthy',
					last_health_http_status: 200,
					config_files: []
				};
				const watchers = [
					{
						id: 1,
						name: 'Commerce API',
						service_name: 'commerce',
						status: 'healthy',
						current_version: 'v2.8.1',
						pending_version: '',
						services: [service],
						paused: false,
						polling_activity: 'active',
						last_poll_status: 'no_update',
						deployment_environment: 'production',
						metadata_url: 'https://github.com/acme/commerce',
						last_error: '',
						auto_deploy: true
					},
					{
						id: 2,
						name: 'Customer portal',
						service_name: 'portal',
						status: 'pending_approval',
						current_version: 'v1.4.0',
						pending_version: 'v1.5.0',
						services: [],
						paused: false,
						polling_activity: 'active',
						last_poll_status: 'intercepted',
						deployment_environment: 'production',
						metadata_url: 'https://github.com/acme/portal',
						last_error: '',
						auto_deploy: false
					},
					{
						id: 3,
						name: 'Background worker',
						service_name: 'worker',
						status: 'failed',
						current_version: 'v3.0.2',
						pending_version: '',
						services: [],
						paused: true,
						polling_activity: 'paused',
						last_poll_status: 'error',
						deployment_environment: 'staging',
						metadata_url: 'https://github.com/acme/worker',
						last_error: 'Health check timed out after deployment.',
						auto_deploy: true
					}
				].map((w) => ({
					...w,
					check_interval_sec: 60,
					max_kept_versions: 3,
					hc_enabled: false,
					hc_url: '',
					notify_version_found: false,
					notify_deployment_succeeded: false,
					notify_deployment_failed: false,
					notify_rollback_succeeded: false,
					notify_rollback_failed: false,
					notify_service_health_changed: false,
					install_dir: 'D:\\apps\\' + w.service_name,
					last_deployed: new Date(Date.now() - 3600000).toISOString(),
					last_poll_at: new Date().toISOString(),
					last_checked: new Date().toISOString(),
					max_ignored_version: '',
					webhook_enabled: false,
					webhook_failure_streak: 0
				}));
				await page.route('**/api/**', async (route) => {
					const url = new URL(route.request().url());
					assert.ok(url.pathname.startsWith(prefix + '/api/'), 'base path preserved');
					const pathname = url.pathname.slice(prefix.length);
					const json = (data, status = 200) =>
						route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(data) });
					if (pathname === '/api/auth/status' || pathname === '/api/auth/login')
						return json({ authenticated: true });
					if (pathname === '/api/auth/bootstrap') return json({ using_default_password: false });
					if (pathname === '/api/self/update-check') return json({ update_available: false });
					if (pathname === '/api/github/inspect')
						return json({
							source: 'native',
							assets: [],
							repo_url: 'https://github.com/acme/commerce',
							services: {},
							release_ref: 'latest'
						});
					if (pathname === '/api/self/version')
						return json({
							version: 'v1.12.0',
							go_version: 'go1.25.6',
							os: 'windows',
							arch: 'amd64',
							executable: 'watcher.exe'
						});
					if (pathname === '/api/self/config')
						return json({
							environment: 'production',
							github_deploy_enabled: false,
							log_dir: 'D:/logs',
							nssm_path: 'nssm.exe',
							db_path: 'watcher.db',
							api_port: '8080',
							api_base_url: '',
							watcher_repo_url: 'https://github.com/fanboykun/watcher',
							watcher_service_name: 'Watcher',
							webhook_default_url: '',
							webhook_timeout_sec: 10,
							webhook_retry_schedule_sec: '0,10',
							webhook_auto_pause_enabled: true,
							webhook_auto_pause_after_failures: 5,
							webhook_event_retention_days: 90,
							webhook_delivery_retention_days: 30,
							env_path: '.env'
						});
					if (pathname === '/api/status')
						return json({
							status: 'running',
							version: 'v1.12.0',
							uptime_human: '4d 12h',
							watcher_count: empty ? 0 : 3,
							service_count: empty ? 0 : 1,
							deploys_24h: 12
						});
					if (pathname === '/api/watchers')
						return failed
							? json({ error: 'Connection interrupted' }, 503)
							: json(empty ? [] : watchers);
					if (pathname === '/api/services') return json(empty ? [] : [service]);
					if (pathname === '/api/services/1/status') return json(service);
					if (pathname === '/api/services/1/stop') {
						serviceActions++;
						return json({ message: 'stopped' });
					}
					if (/^\/api\/watchers\/\d+$/.test(pathname)) {
						const watcher = watchers.find((w) => w.id === Number(pathname.split('/').at(-1)));
						if (route.request().method() === 'PUT') {
							pauses++;
							Object.assign(watcher, route.request().postDataJSON());
							watcher.polling_activity = watcher.paused ? 'paused' : 'active';
						}
						return json(watcher);
					}
					if (pathname === '/api/watchers/1/check') return json({ message: 'Poll requested' });
					if (pathname.endsWith('/events'))
						return route.fulfill({ contentType: 'text/event-stream', body: ': waiting\n\n' });
					if (pathname.endsWith('/versions'))
						return json({ versions: [], downloads: [], current_version: 'v2.8.1' });
					if (/\/(deploys|polls|webhook-deliveries)$/.test(pathname))
						return json({ data: [], total: 0 });
					if (pathname === '/api/services/1') return json({ service, watcher: watchers[0] });
					if (pathname === '/api/services/2')
						return json({
							service: {
								...service,
								id: 2,
								watcher_id: 2,
								windows_service_name: 'portal-api',
								binary_name: 'portal.exe'
							},
							watcher: watchers[1]
						});
					if (pathname.endsWith('/health/history')) return json([]);
					if (pathname === '/api/logs')
						return json({
							source: 'agent',
							lines: [],
							format: 'json',
							missing: false,
							log_file: 'watcher.log'
						});
					return json([]);
				});
				const visible = (text) =>
					page.getByText(text, { exact: true }).first().waitFor({ state: 'visible' });
				const noOverflow = async () =>
					assert.equal(
						await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
						true,
						`${prefix} ${width}px overflow at ${page.url()}`
					);
				await page.goto(origin + prefix + '/');
				await visible('Your fleet, at a glance.');
				await visible('Deployment fleet');
				assert.equal(await page.locator('.attention-row').count(), 2);
				await noOverflow();
				await page.evaluate(() => document.fonts.ready);
				if (process.env.SCREENSHOT_DIR) {
					await fs.mkdir(process.env.SCREENSHOT_DIR, { recursive: true });
					await page.screenshot({
						path: path.join(
							process.env.SCREENSHOT_DIR,
							`overview${prefix ? '-prefix' : ''}-${width}.png`
						),
						fullPage: true,
						animations: 'disabled'
					});
				}
				if (process.env.CAPTURE_ONLY) {
					await page.close();
					continue;
				}
				if (width < 1024) {
					await page.getByRole('button', { name: 'Open navigation' }).click();
					await page
						.getByRole('dialog')
						.getByRole('link', { name: 'Watchers', exact: true })
						.click();
					await page.getByRole('dialog').waitFor({ state: 'hidden' });
				} else
					await page
						.getByRole('navigation')
						.getByRole('link', { name: 'Watchers', exact: true })
						.click();
				await page.getByRole('heading', { name: 'Watchers', exact: true }).waitFor();
				await page.getByLabel('Search repositories…').fill('portal');
				await visible('1 / 3 shown');
				await page.getByLabel('Search repositories…').fill('missing');
				await visible('No matching watchers');
				await page.getByRole('button', { name: 'Clear filters', exact: true }).click();
				await page.getByLabel('Filter watchers by state').selectOption('attention');
				await visible('2 / 3 shown');
				await noOverflow();
				await page.getByLabel('Filter watchers by state').selectOption('all');
				await page
					.getByRole('button', { name: 'Actions for Commerce API' })
					.filter({ visible: true })
					.click();
				await page.getByRole('menuitem', { name: 'Poll now' }).click();
				await visible('Poll requested');
				await page.goto(origin + prefix + '/services');
				await page.getByRole('heading', { name: 'Services', exact: true }).waitFor();
				await page.getByLabel('Filter services by runtime').selectOption('iis');
				await visible('No matching services');
				await page.getByRole('button', { name: 'Clear filters', exact: true }).click();
				await page
					.getByRole('button', { name: 'Actions for commerce-api' })
					.filter({ visible: true })
					.click();
				await page.getByRole('menuitem', { name: 'Stop service' }).click();
				await page.getByRole('dialog').getByRole('button', { name: 'Cancel', exact: true }).click();
				assert.equal(serviceActions, 0, 'cancel must not stop service');
				await noOverflow();
				await page.goto(origin + prefix + '/polling');
				await page.getByRole('heading', { name: 'Polling', exact: true }).waitFor();
				await page.getByLabel('Search polling repositories…').fill('worker');
				await page.getByRole('button', { name: 'Resume', exact: true }).click();
				await visible('Background worker: polling resumed.');
				assert.equal(pauses, 1);
				await noOverflow();
				await page.goto(origin + prefix + '/watchers/1');
				await page.getByRole('heading', { name: 'Commerce API', exact: true }).waitFor();
				await page.getByRole('tab', { name: 'Overview', exact: true }).waitFor();
				await noOverflow();
				await page.goto(origin + prefix + '/services/1');
				await page.getByRole('heading', { name: 'commerce-api', exact: true }).waitFor();
				await noOverflow();
				// The same progress component drives both setup workflows.
				await page.goto(origin + prefix + '/watchers/new');
				await page
					.getByRole('heading', { name: 'Connect. Configure. Deploy.', exact: true })
					.waitFor();
				await page.getByRole('button', { name: 'Next', exact: true }).click();
				await page
					.getByText('Repository URL or metadata URL is required.', { exact: true })
					.waitFor();
				await page
					.getByLabel('GitHub Repository or metadata URL', { exact: true })
					.fill('https://github.com/acme/commerce');
				await page.getByRole('button', { name: 'Next', exact: true }).click();
				await page.getByLabel('Watcher Display Name', { exact: true }).waitFor();
				assert.equal(
					await page.getByLabel('Watcher Display Name', { exact: true }).inputValue(),
					'commerce'
				);
				await page
					.getByRole('navigation', { name: 'Setup progress' })
					.getByRole('button', { name: /Repository/ })
					.click();
				assert.equal(
					await page.getByLabel('GitHub Repository or metadata URL', { exact: true }).inputValue(),
					'https://github.com/acme/commerce'
				);
				await noOverflow();
				await page.goto(origin + prefix + '/services/1/edit');
				await page.getByLabel('Binary Name', { exact: true }).waitFor();
				assert.equal(await page.getByRole('navigation', { name: 'Setup progress' }).count(), 0);
				assert.equal(await page.getByLabel('Binary Name', { exact: true }).inputValue(), 'api.exe');
				await noOverflow();
				async function navigateInApp(href) {
					await page.evaluate((href) => {
						const link = document.createElement('a');
						link.href = href;
						link.textContent = 'Switch fixture resource';
						document.querySelector('main').append(link);
					}, prefix + href);
					await page.getByRole('link', { name: 'Switch fixture resource', exact: true }).click();
				}
				await navigateInApp('/services/2/edit');
				await page.waitForFunction(
					() => document.querySelector('#binaryName')?.value === 'portal.exe'
				);
				await page.goto(origin + prefix + '/watchers/1/edit');
				await page.waitForFunction(
					() =>
						document.querySelector('#editMetadataURL')?.value === 'https://github.com/acme/commerce'
				);
				await navigateInApp('/watchers/2/edit');
				await page.waitForFunction(
					() =>
						document.querySelector('#editMetadataURL')?.value === 'https://github.com/acme/portal'
				);
				await page.goto(origin + prefix + '/watchers/1/services/new');
				await page.getByRole('navigation', { name: 'Setup progress' }).waitFor();
				await noOverflow();
				await page.goto(origin + prefix + '/webhooks');
				await page.getByRole('heading', { name: 'Webhooks', exact: true }).waitFor();
				await noOverflow();
				await page.goto(origin + prefix + '/logs');
				await page.getByRole('heading', { name: 'Agent logs', exact: true }).waitFor();
				await noOverflow();
				await page.goto(origin + prefix + '/settings');
				await page.getByRole('heading', { name: 'Settings', exact: true }).waitFor();
				await page.getByRole('tab', { name: 'Maintenance', exact: true }).click();
				await noOverflow();
				empty = true;
				await page.goto(origin + prefix + '/');
				await visible('Your first release starts here.');
				await page.goto(origin + prefix + '/watchers');
				await visible('Put your first repository on watch.');
				await page.goto(origin + prefix + '/services');
				await visible('A home for every service.');
				empty = false;
				failed = true;
				await page.goto(origin + prefix + '/watchers');
				await page.getByText(/Connection interrupted/).waitFor();
				failed = false;
				await page.getByRole('button', { name: 'Try again', exact: true }).click();
				await visible('3 / 3 shown');
				await page.reload();
				await visible('3 / 3 shown');
				// Sign-out and login preserve the password-backed authentication flow.
				if (width < 1024) await page.getByRole('button', { name: 'Open navigation' }).click();
				await page
					.getByRole('button', { name: 'Sign out', exact: true })
					.filter({ visible: true })
					.click();
				await page.getByLabel('Password', { exact: true }).fill('fixture');
				await page.getByRole('button', { name: 'Open workspace', exact: true }).click();
				await visible('3 / 3 shown');
				assert.deepEqual(errors, []);
				console.log(
					`PASS redesign ${prefix || '/'} ${width}px: dashboard attention, search/filter/reset, actions, cancel, polling, details, resource navigation, empty, retry, refresh, auth, overflow`
				);
				await page.close();
			}
	} catch (error) {
		for (const page of browser.contexts().flatMap((context) => context.pages())) {
			console.error('FAILURE URL', page.url());
			console.error(
				(
					await page
						.locator('main')
						.innerText()
						.catch(() => '')
				).slice(0, 1800)
			);
			if (process.env.SCREENSHOT_DIR)
				await page.screenshot({
					path: path.join(process.env.SCREENSHOT_DIR, 'failure.png'),
					fullPage: true,
					animations: 'disabled'
				});
		}
		throw error;
	} finally {
		await browser.close();
		server.closeAllConnections();
		await new Promise((resolve) => server.close(resolve));
	}
})().catch((error) => {
	console.error(error);
	process.exitCode = 1;
});
