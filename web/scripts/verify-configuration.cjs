/* eslint-disable @typescript-eslint/no-require-imports -- Standalone CommonJS browser verification. */
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
				page.setDefaultTimeout(10000);
				const errors = [];
				page.on('pageerror', (e) => errors.push(e.message));
				await page.addInitScript(() => localStorage.setItem('watcher.auth.password', 'test'));
				const env = '# keep this comment\r\nTOKEN="hello=world # quoted"\r\nPORT=80\nPORT=81\r\n';
				const activeFile = {
					file_path: 'appsettings.json',
					target: 'app_dir',
					content: '{\n  "active": true\n}\n'
				};
				let service = {
					id: 1,
					watcher_id: 1,
					windows_service_name: 'API service',
					binary_name: 'api.exe',
					service_type: 'nssm',
					env_file: '.env',
					env_content: env,
					config_files: [activeFile],
					last_service_status: 'running',
					last_health_status: 'healthy',
					last_health_http_status: 200,
					health_check_url: 'http://localhost:9000/health',
					public_url: '',
					start_arguments: '',
					iis_app_kind: 'static',
					iis_app_pool: '',
					iis_site_name: ''
				};
				const watcher = {
					id: 1,
					name: 'Commerce',
					service_name: 'commerce',
					status: 'pending_approval',
					metadata_url: 'https://github.com/owner/repo',
					current_version: 'v1',
					pending_version: 'v2',
					pending_catalog_id: 7,
					services: [service],
					install_dir: 'D:/apps/commerce',
					check_interval_sec: 60,
					paused: true,
					polling_activity: 'paused',
					last_error: '',
					auto_deploy: false,
					max_ignored_version: '',
					webhook_failure_streak: 0
				};
				let failSave = false,
					failCandidate = false,
					swapOnSave = false,
					liveWrites = [],
					candidateWrites = [],
					approvals = [],
					starts = 0,
					restarts = 0,
					relatedFetches = 0;
				const drafts = new Map();
				const attempts = [
					{
						id: 42,
						watcher_id: 1,
						kind: 'rollback',
						reason: 'auto_after_deploy_failure',
						version: 'v1',
						from_version: 'v2',
						failed_target_version: 'v2',
						status: 'succeeded',
						triggered_by: 'agent',
						parent_attempt_id: 41,
						root_attempt_id: 41,
						started_at: '2026-10-23T05:02:00Z',
						duration_ms: 1200,
						completed_at: '2026-10-23T05:02:02Z',
						error: ''
					},
					{
						id: 43,
						watcher_id: 1,
						kind: 'deploy',
						reason: 'catalog_release',
						version: 'v3',
						from_version: 'v1',
						status: 'succeeded',
						triggered_by: 'manual',
						started_at: '2026-10-23T06:00:00Z',
						duration_ms: 500,
						completed_at: '2026-10-23T06:00:01Z',
						error: ''
					}
				];
				const parentAttempt = {
					id: 41,
					watcher_id: 1,
					kind: 'deploy',
					reason: 'catalog_release',
					version: 'v2',
					from_version: 'v1',
					status: 'failed',
					triggered_by: 'manual',
					failure_phase: 'health_check',
					error: 'Health check timed out',
					started_at: '2026-10-23T05:01:00Z',
					duration_ms: 500
				};
				await page.route('**/api/**', async (route) => {
					const url = new URL(route.request().url());
					assert.ok(url.pathname.startsWith(prefix + '/api/'));
					const path = url.pathname.slice(prefix.length),
						method = route.request().method();
					const json = (body, status = 200) =>
						route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
					if (path === '/api/auth/status') return json({ authenticated: true });
					if (path === '/api/self/update-check') return json({ update_available: false });
					if (path === '/api/services/1') return json({ service, watcher });
					if (path === '/api/services/1/status') return json(service);
					if (path === '/api/services/1/start') {
						starts++;
						return json({ message: 'Service started' });
					}
					if (path === '/api/services/1/restart') {
						restarts++;
						return json({ message: 'Service restarted' });
					}
					if (path === '/api/watchers/1/services/1' && method === 'PUT') {
						if (failSave) return json({ error: 'Disk could not be written' }, 500);
						const body = route.request().postDataJSON();
						liveWrites.push(body);
						service = { ...service, ...body };
						watcher.services = [service];
						return json(service);
					}
					if (path === '/api/watchers/1') return json(watcher);
					if (path === '/api/watchers') return json([watcher]);
					if (path === '/api/services') return json([{ ...service, watcher_name: watcher.name }]);
					if (path.endsWith('/health/history')) return json([]);
					if (path === '/api/services/1/logs') return json({ lines: [] });
					if (path.endsWith('/events'))
						return route.fulfill({ contentType: 'text/event-stream', body: ': waiting\n\n' });
					if (path === '/api/watchers/1/candidate') {
						if (method === 'PUT') {
							if (failCandidate) return json({ error: 'Draft save failed' }, 503);
							const body = route.request().postDataJSON();
							candidateWrites.push(body);
							drafts.set(body.target_version, body.services[0]);
							if (swapOnSave) watcher.pending_catalog_id = 8;
							return json({ message: 'Draft saved' });
						}
						const target = url.searchParams.get('target') || 'v2',
							draft = drafts.get(target);
						return json({
							target_version: target,
							status: watcher.status,
							saved_targets: [...drafts.keys()],
							services: [
								{
									service_id: 1,
									service_name: 'API service',
									service_type: 'nssm',
									active_env: service.env_content,
									candidate_env: draft?.env_content ?? service.env_content,
									has_candidate_env: !!draft,
									candidate_target: draft ? target : '',
									active_config_files: service.config_files,
									candidate_config_files: draft?.config_files ?? service.config_files,
									has_candidate_config_files: !!draft && Object.hasOwn(draft, 'config_files')
								}
							]
						});
					}
					if (path === '/api/watchers/1/releases/7/deploy') {
						if (watcher.pending_catalog_id !== 7)
							return json({ error: 'A different catalog candidate is selected' }, 409);
						approvals.push(7);
						watcher.status = 'deploying';
						return json({ message: 'queued', deploy_log_id: 43 }, 202);
					}
					if (path.endsWith('/versions')) return json({ versions: [], downloads: [] });
					if (path.endsWith('/deploys')) {
						const second = url.searchParams.get('page') === '2';
						return json({
							data: second ? [parentAttempt] : attempts,
							total: 11,
							page: second ? 2 : 1,
							pageSize: 10
						});
					}
					if (path === '/api/watchers/1/deploys/41') {
						relatedFetches++;
						return json(parentAttempt);
					}
					if (path.endsWith('/polls') || path.endsWith('/webhook-deliveries'))
						return json({ data: [], total: 0 });
					return json({});
				});
				const visit = (route) => page.goto(origin + prefix + route);
				const editor = () =>
					page.getByRole('textbox', { name: 'API service .env contents', exact: true });
				const workspace = () =>
					page.getByRole('region', { name: 'API service configuration workspace', exact: true });
				async function type(locator, value) {
					await locator.click();
					await locator.press('ControlOrMeta+a');
					await locator.evaluate((node, text) => {
						const clipboardData = new DataTransfer();
						clipboardData.setData('text/plain', text);
						node.dispatchEvent(
							new ClipboardEvent('paste', { clipboardData, bubbles: true, cancelable: true })
						);
					}, value);
				}
				async function noOverflow() {
					assert.ok(
						await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
						`${width}px overflow at ${page.url()}`
					);
				}
				const waitUntil = (predicate) =>
					new Promise((resolve, reject) => {
						const start = Date.now();
						const timer = setInterval(() => {
							if (predicate()) {
								clearInterval(timer);
								resolve();
							} else if (Date.now() - start > 10000) {
								clearInterval(timer);
								reject(new Error('Fixture mutation timed out'));
							}
						}, 20);
					});
				await visit('/services/1?tab=env');
				await editor().waitFor();
				await workspace().getByRole('button', { name: 'appsettings.json', exact: true }).click();
				const jsonEditor = page.getByRole('textbox', {
					name: 'API service appsettings.json contents',
					exact: true
				});
				await type(jsonEditor, '{"active":false}');
				await workspace().getByRole('button', { name: 'Review changes', exact: true }).click();
				await page.locator('.cm-deletedChunk').first().waitFor();
				await workspace().getByRole('button', { name: 'Edit', exact: true }).click();
				await workspace().getByRole('button', { name: 'Apply changes', exact: true }).click();
				await page
					.getByRole('dialog')
					.getByRole('button', { name: 'Apply changes', exact: true })
					.click();
				await waitUntil(() => liveWrites.length === 1);
				assert.equal(
					liveWrites[0].env_content,
					env,
					'editing JSON must preserve env bytes, duplicate keys, quotes and CRLF'
				);
				await page.getByRole('dialog').waitFor({ state: 'hidden' });
				await workspace().getByRole('button', { name: '.env', exact: true }).click();
				const draftEnv = '# local draft\nPORT=9000\n';
				await type(editor(), draftEnv);
				await editor().press('ControlOrMeta+z');
				assert.notEqual(await editor().innerText(), draftEnv.trimEnd());
				await editor().press('ControlOrMeta+Shift+z');
				await page.getByRole('tab', { name: 'Health', exact: true }).click();
				await page.getByRole('tab', { name: 'Files', exact: true }).click();
				assert.match(await editor().innerText(), /PORT=9000/);
				await page.getByRole('button', { name: 'Actions', exact: true }).click();
				await page.getByRole('menuitem', { name: 'Start service', exact: true }).click();
				await waitUntil(() => starts === 1);
				await page.getByText('Service started', { exact: true }).waitFor();
				assert.match(
					await editor().innerText(),
					/PORT=9000/,
					'service action refresh preserves draft'
				);
				const editLink = page.getByRole('link', { name: 'Edit', exact: true });
				await editLink.click();
				await page
					.getByRole('dialog')
					.getByRole('button', { name: 'Keep editing', exact: true })
					.click();
				assert.match(page.url(), /services\/1\?tab=env/);
				failSave = true;
				await workspace().getByRole('button', { name: 'Apply changes', exact: true }).click();
				await page
					.getByRole('dialog')
					.getByRole('button', { name: 'Apply changes', exact: true })
					.click();
				await page
					.getByRole('dialog')
					.getByText('Disk could not be written', { exact: false })
					.waitFor();
				assert.match(await editor().innerText(), /PORT=9000/);
				assert.equal(liveWrites.length, 1);
				failSave = false;
				await page
					.getByRole('dialog')
					.getByRole('button', { name: 'Keep editing', exact: true })
					.click();
				await workspace().getByRole('button', { name: 'Apply and restart', exact: true }).click();
				await page
					.getByRole('dialog')
					.getByRole('button', { name: 'Apply and restart', exact: true })
					.click();
				await waitUntil(() => restarts === 1 && liveWrites.length === 2);
				await page.getByRole('dialog').waitFor({ state: 'hidden' });
				assert.equal(liveWrites[1].env_content, draftEnv.replaceAll('\n', '\r\n'));
				if (process.env.SCREENSHOT_DIR) {
					await fs.mkdir(process.env.SCREENSHOT_DIR, { recursive: true });
					await page.screenshot({
						path: path.join(
							process.env.SCREENSHOT_DIR,
							`configuration-${prefix ? 'prefix-' : ''}${width}.png`
						),
						fullPage: true,
						animations: 'disabled'
					});
				}
				await noOverflow();
				await visit('/services/1/edit');
				await page.getByLabel('Binary Name', { exact: true }).waitFor();
				assert.equal(
					await page.getByRole('navigation', { name: 'Setup progress' }).count(),
					0,
					'existing services use a single page'
				);
				await page.getByLabel('Binary Name', { exact: true }).fill('server.exe');
				await page.getByRole('button', { name: 'Save Service Changes', exact: true }).click();
				await page.getByText('Service configuration saved.', { exact: true }).waitFor();
				assert.equal(liveWrites.at(-1).binary_name, 'server.exe');
				await noOverflow();
				await page.getByLabel('Binary Name', { exact: true }).fill('discarded.exe');
				await page.getByRole('link', { name: 'Back to service', exact: true }).click();
				await page
					.getByRole('dialog')
					.getByRole('button', { name: 'Discard and leave', exact: true })
					.click();
				await page.waitForURL(origin + prefix + '/services/1');
				assert.equal(service.binary_name, 'server.exe');
				await visit('/watchers/1?tab=candidates');
				await editor().waitFor();
				const writesBeforeCandidates = liveWrites.length;
				await type(editor(), '# staged only\nPORT=9100\nPORT=9200\n');
				await page.getByRole('button', { name: 'Next release', exact: true }).click();
				await page
					.getByRole('dialog')
					.getByRole('button', { name: 'Keep editing', exact: true })
					.click();
				assert.match(await editor().innerText(), /PORT=9200/);
				await page.getByRole('button', { name: 'Save candidate', exact: true }).click();
				await waitUntil(() => candidateWrites.length === 1);
				assert.equal(
					Object.hasOwn(candidateWrites[0].services[0], 'config_files'),
					false,
					'env-only candidate must inherit files'
				);
				await workspace().getByRole('button', { name: 'Stage active files', exact: true }).click();
				await workspace().getByRole('button', { name: 'appsettings.json', exact: true }).click();
				await type(jsonEditor, '{"candidate":true}');
				await workspace().getByRole('button', { name: 'Review changes', exact: true }).click();
				await page.locator('.cm-deletedChunk').first().waitFor();
				await page.getByRole('button', { name: 'Save candidate', exact: true }).click();
				await waitUntil(() => candidateWrites.length === 2);
				assert.equal(candidateWrites[1].services[0].config_files[0].content, '{"candidate":true}');
				assert.equal(liveWrites.length, writesBeforeCandidates);
				await page.getByRole('button', { name: 'Review deployment', exact: true }).click();
				await page
					.getByRole('dialog')
					.getByRole('heading', { name: 'Deploy reviewed release v2?', exact: true })
					.waitFor();
				assert.equal(approvals.length, 0);
				await page
					.getByRole('dialog')
					.getByRole('button', { name: 'Back to configuration', exact: true })
					.click();
				await workspace().getByRole('button', { name: '.env', exact: true }).click();
				await workspace().getByRole('button', { name: 'Edit', exact: true }).click();
				await type(editor(), 'PORT=9300\n');
				failCandidate = true;
				await page.getByRole('button', { name: 'Save candidate', exact: true }).click();
				await page.getByText(/Draft save failed/).waitFor();
				assert.match(await editor().innerText(), /9300/);
				failCandidate = false;
				swapOnSave = true;
				await page.getByRole('button', { name: 'Review deployment', exact: true }).click();
				await page
					.getByRole('dialog')
					.getByRole('button', { name: 'Deploy reviewed release', exact: true })
					.click();
				await page
					.getByRole('dialog')
					.getByText(/A different catalog candidate is selected/)
					.waitFor();
				assert.equal(
					approvals.length,
					0,
					'approval must retain exact artifact identity during save'
				);
				await page
					.getByRole('dialog')
					.getByRole('button', { name: 'Back to configuration', exact: true })
					.click();
				swapOnSave = false;
				watcher.pending_catalog_id = 7;
				await page.reload();
				await editor().waitFor();
				await page.getByRole('button', { name: 'Review deployment', exact: true }).click();
				await page
					.getByRole('dialog')
					.getByRole('button', { name: 'Deploy reviewed release', exact: true })
					.click();
				await waitUntil(() => approvals.length === 1);
				await page.getByRole('dialog').waitFor({ state: 'hidden' });
				assert.equal(liveWrites.length, writesBeforeCandidates);
				watcher.status = 'healthy';
				await visit('/watchers/1?tab=deploys');
				await page.getByRole('article', { name: 'Attempt 42', exact: true }).waitFor();
				assert.deepEqual(
					await page
						.locator('.attempt-card')
						.evaluateAll((nodes) => nodes.map((node) => node.getAttribute('aria-label'))),
					['Attempt 43', 'Attempt 42']
				);
				await page
					.getByRole('article', { name: 'Attempt 42', exact: true })
					.getByRole('button', { name: 'Related attempts' })
					.click();
				await page.getByText(/deploy · v2 ·/).waitFor();
				assert.equal(relatedFetches, 1, 'fetch a missing parent only once');
				await page
					.getByRole('article', { name: 'Attempt 42', exact: true })
					.getByRole('button', { name: 'Related attempts' })
					.click();
				await page
					.getByRole('article', { name: 'Attempt 42', exact: true })
					.getByRole('button', { name: 'Related attempts' })
					.click();
				assert.equal(relatedFetches, 1);
				await page.getByRole('button', { name: 'Next', exact: true }).click();
				await page.getByRole('article', { name: 'Attempt 41', exact: true }).waitFor();
				await noOverflow();
				await visit('/watchers/1?tab=candidates');
				await page.getByRole('button', { name: 'Specific version', exact: true }).click();
				await editor().and(page.locator('[aria-readonly="true"]')).waitFor();
				await page.getByLabel('Candidate target version', { exact: true }).fill('v9');
				await page.getByRole('button', { name: 'Load', exact: true }).click();
				await editor().and(page.locator('[aria-readonly="false"]')).waitFor();
				await type(editor(), 'TARGET=loaded-v9');
				await page.getByLabel('Candidate target version', { exact: true }).fill('v10');
				await page.getByRole('button', { name: 'Save candidate', exact: true }).click();
				await waitUntil(() => candidateWrites.at(-1).target_version === 'v9');
				assert.equal(candidateWrites.at(-1).services[0].env_content, 'TARGET=loaded-v9');
				await page.getByRole('button', { name: 'Load', exact: true }).click();
				await workspace().getByText('v10', { exact: true }).waitFor();
				assert.match(await editor().innerText(), /PORT=9000/);
				assert.deepEqual(errors, []);
				console.log(
					`PASS configuration ${prefix || '/'} ${width}px: real editor, CRLF, undo, review, draft survival, failed-save retry, apply/restart, navigation guard, single-page settings, candidate inheritance/isolation, exact-artifact approval, attempt links/pagination`
				);
				await page.close();
			}
	} catch (error) {
		for (const page of browser.contexts().flatMap((context) => context.pages())) {
			console.error('FAILED', page.url());
			console.error(
				(
					await page
						.locator('main')
						.innerText()
						.catch(() => '')
				).slice(-2000)
			);
			if (process.env.SCREENSHOT_DIR) {
				await fs.mkdir(process.env.SCREENSHOT_DIR, { recursive: true });
				await page.screenshot({
					path: path.join(process.env.SCREENSHOT_DIR, 'failure.png'),
					fullPage: true,
					animations: 'disabled'
				});
			}
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
