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
	let browser;
	try {
		browser = await chromium.launch({
			headless: true,
			executablePath: process.env.CHROMIUM_EXECUTABLE || undefined
		});
		for (const prefix of ['', '/watcher'])
			for (const viewport of [
				{ width: 1440, height: 1000 },
				{ width: 390, height: 844 }
			]) {
				const page = await browser.newPage({ viewport });
				const errors = [];
				page.on('pageerror', (error) => {
					errors.push(error.message);
					if (process.env.DEBUG_CATALOG) console.log('PAGE ERROR', error.message);
				});
				await page.addInitScript(() => localStorage.setItem('watcher.auth.password', 'test'));
				const watcher = {
					id: 1,
					name: 'Catalog recovery',
					service_name: 'api',
					metadata_url: 'https://github.com/owner/repo',
					release_ref: 'latest',
					status: 'healthy',
					current_version: 'v1',
					pending_catalog_id: 0,
					pending_version: '',
					last_error: '',
					max_ignored_version: '',
					services: [],
					polling_activity: 'paused',
					paused: true,
					auto_deploy: true,
					intercept_next_release: false,
					check_interval_sec: 60,
					install_dir: 'D:\\apps\\api',
					last_poll_status: '',
					last_checked: null,
					last_poll_at: null,
					last_deployed: null,
					webhook_enabled: false,
					webhook_failure_streak: 0
				};
				let downloads = [];
				let approvals = 0;
				let deployHistory = [];
				let discardCalls = 0;
				let eventBody = ': idle\n\n';
				let finishDownload;
				let failCandidate = false;
				let delayedNext = false;
				let releaseNext;
				let saveSwitchesArtifact = false;
				let rejectedWrongArtifact = 0;
				let failCatalog = false;
				let emptyCatalog = false;
				let managedMode = false;
				const managedDrafts = new Map();
				const candidateWrites = [];
				let liveConfigWrites = 0;
				const activeFiles = [
					{ file_path: 'appsettings.json', target: 'app_dir', content: '{"active":true}' }
				];
				const release = {
					id: 11,
					name: 'Release two',
					tag_name: 'v2',
					body: 'Deployment notes',
					html_url: 'https://github.com/owner/repo/releases/tag/v2',
					published_at: '2026-10-09T12:00:00Z',
					assets: [
						{ id: 22, name: 'api.zip', size: 123 },
						{ id: 23, name: 'worker.zip', size: 456 }
					],
					prerelease: false,
					draft: false
				};
				await page.route('**/api/**', async (route) => {
					const url = new URL(route.request().url());
					assert.ok(
						url.pathname.startsWith(prefix + '/api/'),
						`API escaped dashboard prefix: ${url.pathname}`
					);
					const path = url.pathname.slice(prefix.length);
					if (process.env.DEBUG_CATALOG) console.log(route.request().method(), path, url.search);
					const respond = (data, status = 200) =>
						route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(data) });
					if (managedMode && /\/services\//.test(path) && route.request().method() !== 'GET') {
						liveConfigWrites++;
						return respond({ error: 'Candidate editing must not write live configuration' }, 500);
					}
					if (path === '/api/auth/status') return respond({ authenticated: true });
					if (path === '/api/self/update-check') return respond({ update_available: false });
					if (path === '/api/watchers/1') return respond(watcher);
					if (path.endsWith('/events'))
						return route.fulfill({ contentType: 'text/event-stream', body: eventBody });
					if (path.endsWith('/versions'))
						return respond({ versions: [], downloads, current_version: watcher.current_version });
					if (path.endsWith('/releases/download')) {
						await new Promise((resolve) => {
							finishDownload = resolve;
						});
						downloads = [
							{
								id: 7,
								watcher_id: 1,
								release_id: 11,
								asset_id: 22,
								version: 'v2',
								tag: 'v2',
								asset_name: 'api.zip',
								downloaded_at: '2026-10-09T12:00:00Z'
							}
						];
						return respond(downloads[0], 201);
					}
					if (path.endsWith('/releases')) {
						if (failCatalog) return respond({ error: 'GitHub unavailable' }, 502);
						const catalogPage = Number(url.searchParams.get('page') || 1);
						const olderReleases = [
							{
								...release,
								id: 12,
								name: 'Older prerelease',
								tag_name: 'v1.0.0-rc1',
								prerelease: true
							},
							{
								...release,
								id: 13,
								name: 'Unpublished release',
								tag_name: 'v3-draft',
								draft: true,
								published_at: ''
							},
							{ ...release, id: 14, name: 'Source release', tag_name: 'source-only', assets: [] }
						];
						return respond({
							repository: 'owner/repo',
							releases: emptyCatalog ? [] : catalogPage === 1 ? [release] : olderReleases,
							downloads,
							page: catalogPage,
							has_next: !emptyCatalog && catalogPage === 1
						});
					}
					if (path.endsWith('/releases/7/candidate')) {
						Object.assign(watcher, {
							pending_version: 'v2',
							pending_catalog_id: 7,
							status: 'pending_approval'
						});
						return respond({ version: 'v2', catalog_id: 7 });
					}
					if (path.endsWith('/candidate')) {
						if (route.request().method() === 'PUT') {
							if (managedMode) {
								const body = route.request().postDataJSON();
								const draft = body.services[0];
								if (draft.config_files?.some((file) => file.file_path.includes('..')))
									return respond(
										{ error: 'Managed config file path must stay within its target directory' },
										400
									);
								candidateWrites.push(body);
								managedDrafts.set(body.target_version, {
									...managedDrafts.get(body.target_version),
									...draft
								});
							}
							if (saveSwitchesArtifact) watcher.pending_catalog_id = 8;
							return respond({ message: 'Saved' });
						}
						if (failCandidate) return respond({ error: 'Candidate unavailable' }, 503);
						const target = url.searchParams.get('target');
						if (target === 'next' && delayedNext)
							await new Promise((resolve) => {
								releaseNext = resolve;
							});
						return respond({
							target_version: target,
							status: watcher.status,
							services: [
								{
									service_id: 1,
									service_name: 'API service',
									service_type: 'nssm',
									active_env: 'TARGET=active',
									candidate_env: managedMode
										? (managedDrafts.get(target)?.env_content ?? `TARGET=${target}`)
										: `TARGET=${target}`,
									has_candidate_env: true,
									active_config_files: managedMode ? activeFiles : [],
									candidate_config_files: managedMode
										? (managedDrafts.get(target)?.config_files ?? activeFiles)
										: [],
									has_candidate_config_files:
										managedMode && Array.isArray(managedDrafts.get(target)?.config_files)
								}
							]
						});
					}
					if (path.endsWith('/releases/7/deploy')) {
						if (watcher.pending_catalog_id !== 7) {
							rejectedWrongArtifact++;
							return respond({ error: 'A different catalog candidate is selected' }, 409);
						}
						approvals++;
						deployHistory = [
							{
								id: 42,
								watcher_id: 1,
								version: 'v2',
								from_version: 'v1',
								status: 'failed',
								triggered_by: 'manual',
								started_at: '2026-10-09T12:00:00Z',
								duration_ms: 100,
								error: 'Staged artifact checksum changed; download again',
								logs: 'Validation failed'
							}
						];
						Object.assign(watcher, {
							status: 'failed',
							last_error: 'Staged artifact checksum changed; download again'
						});
						return respond({ message: 'Release queued', deploy_log_id: 42 }, 202);
					}
					if (path.endsWith('/discard')) {
						discardCalls++;
						Object.assign(watcher, {
							pending_version: '',
							pending_catalog_id: 0,
							status: 'healthy'
						});
						return respond({ message: 'Release candidate discarded' });
					}
					if (path.endsWith('/deploys'))
						return respond({ data: deployHistory, total: deployHistory.length });
					if (/\/(polls|webhook-deliveries)$/.test(path)) return respond({ data: [], total: 0 });
					return respond({});
				});
				await page.goto(`${origin}${prefix}/watchers/1?tab=releases`);
				await page.getByRole('combobox', { name: 'Artifact for v2' }).selectOption('23');
				assert.equal(
					await page.getByRole('combobox', { name: 'Artifact for v2' }).inputValue(),
					'23'
				);
				await page.getByRole('combobox', { name: 'Artifact for v2' }).selectOption('22');
				await page.getByRole('button', { name: 'Next', exact: true }).click();
				await page
					.getByRole('article', { name: 'Release v1.0.0-rc1', exact: true })
					.getByText('Prerelease', { exact: true })
					.waitFor();
				await page
					.getByRole('article', { name: 'Release source-only', exact: true })
					.getByText('No deployable ZIP assets', { exact: true })
					.waitFor();
				await page.getByRole('button', { name: 'Actions for v3-draft' }).click();
				assert.equal(
					await page
						.getByRole('menuitem', { name: 'Download artifact' })
						.getAttribute('aria-disabled'),
					'true'
				);
				await page.keyboard.press('Escape');
				assert.equal(
					await page.getByRole('button', { name: 'Next', exact: true }).isEnabled(),
					false
				);
				await page.getByRole('button', { name: 'Previous', exact: true }).click();
				await page
					.getByRole('article', { name: 'Release v2', exact: true })
					.getByText('Release notes', { exact: true })
					.click();
				await page.getByText('Deployment notes', { exact: true }).waitFor();
				emptyCatalog = true;
				await page.getByRole('button', { name: 'Refresh', exact: true }).click();
				await page.getByText('No releases on this page.', { exact: true }).waitFor();
				failCatalog = true;
				await page.getByRole('button', { name: 'Refresh', exact: true }).click();
				await page.getByRole('alert').getByText('GitHub unavailable', { exact: true }).waitFor();
				failCatalog = false;
				emptyCatalog = false;
				await page.getByRole('button', { name: 'Refresh', exact: true }).click();
				await page.getByRole('button', { name: 'Actions for v2' }).click();
				const downloading = page.waitForRequest((request) =>
					request.url().endsWith('/releases/download')
				);
				await page.getByRole('menuitem', { name: 'Download artifact' }).click();
				await downloading;
				// Completion must update the parent even after leaving the releases tab.
				await page.getByRole('tab', { name: /^Versions/ }).click();
				finishDownload();
				await page.getByRole('tab', { name: 'Versions (1)', exact: true }).waitFor();
				await page.getByRole('button', { name: 'Actions for downloaded v2' }).waitFor();
				await page.getByRole('tab', { name: 'GitHub releases', exact: true }).click();
				await page.getByRole('button', { name: 'Actions for v2' }).click();
				await page.getByRole('menuitem', { name: 'Configure candidate' }).click();
				await page.getByRole('button', { name: 'Approve & deploy' }).click();
				await page
					.getByRole('tabpanel', { name: 'Candidates', exact: true })
					.getByText('Staged artifact checksum changed; download again', { exact: true })
					.waitFor();
				assert.equal(approvals, 1);
				await page.getByRole('tab', { name: 'Deploys (1)', exact: true }).click();
				const historyPanel = page.getByRole('tabpanel', { name: 'Deploys (1)', exact: true });
				await historyPanel
					.getByText('Staged artifact checksum changed; download again', { exact: true })
					.filter({ visible: true })
					.waitFor();
				assert.ok(
					(
						await historyPanel
							.locator('a')
							.evaluateAll((links) => links.map((link) => new URL(link.href).pathname))
					).every((href) => href.startsWith(prefix + '/watchers/1/logs/42'))
				);
				await page.getByRole('tab', { name: 'Overview', exact: true }).click();
				await page.getByRole('button', { name: 'Review candidate' }).click();
				await page.getByRole('button', { name: 'Discard', exact: true }).click();
				await page
					.getByRole('dialog')
					.getByRole('button', { name: 'Discard Release', exact: true })
					.click();
				await page.getByRole('tab', { name: /^Versions/ }).click();
				await page.getByText('api.zip', { exact: true }).waitFor();
				assert.equal(discardCalls, 1);
				assert.equal(downloads.length, 1);
				await page.getByRole('button', { name: 'Actions for downloaded v2' }).click();
				await page.getByRole('menuitem', { name: 'Configure candidate' }).click();
				await page.waitForURL(`${origin}${prefix}/watchers/1?tab=candidates`);
				await page
					.getByRole('button', { name: 'Approve & deploy' })
					.and(page.locator(':enabled'))
					.waitFor();
				await page.reload();
				await page.getByRole('button', { name: 'Approve & deploy' }).waitFor();
				await page
					.getByRole('button', { name: 'Approve & deploy' })
					.and(page.locator(':enabled'))
					.waitFor();
				const editor = page.getByRole('tabpanel', { name: 'Candidates', exact: true });
				const pendingPill = editor.locator('button').filter({ hasText: /v2\s+Candidate/ });
				// A failed reload of an already loaded target must never revive old approval.
				failCandidate = true;
				await pendingPill.click();
				await editor.getByText('Candidate unavailable', { exact: true }).waitFor();
				assert.equal(
					await page.getByRole('button', { name: 'Approve & deploy' }).isEnabled(),
					false
				);
				failCandidate = false;

				await pendingPill.click();
				await editor.locator('textarea').waitFor();
				await page
					.getByRole('button', { name: 'Approve & deploy' })
					.and(page.locator(':enabled'))
					.waitFor();
				if (process.env.CATALOG_STRESS === '1') {
					// Dispatching the second click deliberately overlaps requests; the
					// regular acceptance journey above uses physical browser clicks.
					delayedNext = true;
					const nextRequest = page.waitForRequest((request) =>
						request.url().includes('/candidate?target=next')
					);
					await editor.getByRole('button', { name: 'Next release', exact: true }).click();
					await nextRequest;
					await pendingPill.dispatchEvent('click');
					await editor.locator('textarea').waitFor();
					assert.equal(await editor.locator('textarea').inputValue(), 'TARGET=v2');
					const nextResponse = page.waitForResponse((response) =>
						response.url().includes('/candidate?target=next')
					);
					releaseNext();
					delayedNext = false;
					await nextResponse;
					assert.equal(await editor.locator('textarea').inputValue(), 'TARGET=v2');
				}
				// Remote selection can change without a client refresh during save. The
				// request must still name artifact 7 and receive server conflict for 8.
				saveSwitchesArtifact = true;
				await editor.locator('textarea').fill('TARGET=edited');
				await page.getByRole('button', { name: 'Approve & deploy' }).click();
				await page.waitForFunction(() =>
					document
						.querySelector('[role=tabpanel][data-state=active]')
						?.textContent?.includes('A different catalog candidate is selected')
				);
				assert.equal(rejectedWrongArtifact, 1);
				assert.equal(approvals, 1);
				assert.equal(await editor.locator('textarea').inputValue(), 'TARGET=edited');
				saveSwitchesArtifact = false;
				eventBody = 'data: {"type":"status_changed"}\n\n';
				await editor.getByText(/Load the pending candidate using its version below/).waitFor();
				assert.equal(await editor.locator('textarea').inputValue(), 'TARGET=edited');
				assert.equal(
					await page.getByRole('button', { name: 'Approve & deploy' }).isEnabled(),
					false
				);
				await page.getByRole('tab', { name: 'Overview', exact: true }).click();
				await page.getByRole('tab', { name: 'Candidates', exact: true }).click();
				await editor.getByText(/The pending release changed. Your edits are preserved/).waitFor();
				assert.equal(await editor.locator('textarea').inputValue(), 'TARGET=edited');
				eventBody = ': idle\n\n';
				watcher.pending_catalog_id = 7;
				// A successful automatic rollback leaves the selected target available for review.
				Object.assign(watcher, { status: 'healthy', pending_catalog_id: 7, pending_version: 'v2' });
				await page.goto(`${origin}${prefix}/watchers/1?tab=candidates`);
				await page.getByRole('button', { name: 'Discard', exact: true }).waitFor();
				await page.getByRole('button', { name: 'Approve & deploy' }).waitFor();
				for (const status of ['approved', 'deploying']) {
					watcher.status = status;
					await page.reload();
					await page.getByText('Candidate configuration', { exact: true }).waitFor();
					assert.equal(await page.getByRole('button', { name: 'Discard', exact: true }).count(), 0);
					assert.equal(await page.getByRole('button', { name: 'Approve & deploy' }).count(), 0);
				}
				// Managed files stage per target and never call the active service endpoints.
				managedMode = true;
				watcher.status = 'pending_approval';
				await page.reload();
				const filesEditor = page.getByRole('region', { name: 'API service managed config files' });
				await filesEditor
					.getByText('Inherits active files at deployment.', { exact: false })
					.waitFor();
				await editor.locator('textarea').first().fill('TARGET=env-only');
				await editor.getByRole('button', { name: 'Save candidate', exact: true }).click();
				await editor
					.getByText('Candidate configuration saved for version "v2"!', { exact: true })
					.waitFor();
				assert.equal(Object.hasOwn(candidateWrites.at(-1).services[0], 'config_files'), false);
				await filesEditor.getByRole('button', { name: 'Stage active files', exact: true }).click();
				await filesEditor
					.getByLabel('Config file 1 contents', { exact: true })
					.fill('{"candidate":true}');
				await filesEditor.getByRole('button', { name: 'Add config file', exact: true }).click();
				await filesEditor.getByLabel('Config file 2 path', { exact: true }).fill('web.config');
				await filesEditor
					.getByLabel('Config file 2 target', { exact: true })
					.selectOption('release_dir');
				await filesEditor
					.getByLabel('Config file 2 contents', { exact: true })
					.fill('<configuration />');
				await editor.getByText('Unsaved edits', { exact: true }).waitFor();
				await editor.getByRole('button', { name: 'Save candidate', exact: true }).click();
				await editor.getByText('Unsaved edits', { exact: true }).waitFor({ state: 'hidden' });
				assert.deepEqual(managedDrafts.get('v2').config_files, [
					{ file_path: 'appsettings.json', target: 'app_dir', content: '{"candidate":true}' },
					{ file_path: 'web.config', target: 'release_dir', content: '<configuration />' }
				]);
				assert.equal(activeFiles[0].content, '{"active":true}');
				await page.reload();
				await filesEditor.getByLabel('Config file 2 contents', { exact: true }).waitFor();
				assert.equal(
					await filesEditor.getByLabel('Config file 2 target', { exact: true }).inputValue(),
					'release_dir'
				);
				await filesEditor.getByText('Review active file', { exact: true }).first().click();
				await filesEditor.getByText('{"active":true}', { exact: true }).waitFor();
				await editor.getByRole('button', { name: 'Next release', exact: true }).click();
				await filesEditor.getByLabel('Config file 1 contents', { exact: true }).waitFor();
				await filesEditor
					.getByRole('button', { name: 'Remove config file 1', exact: true })
					.click();
				await editor.getByRole('button', { name: 'Save candidate', exact: true }).click();
				await editor.getByText('Unsaved edits', { exact: true }).waitFor({ state: 'hidden' });
				assert.deepEqual(managedDrafts.get('next').config_files, []);
				await pendingPill.click();
				await filesEditor.getByLabel('Config file 2 contents', { exact: true }).waitFor();
				await filesEditor.getByLabel('Config file 1 path', { exact: true }).fill('../outside.json');
				await editor.getByRole('button', { name: 'Save candidate', exact: true }).click();
				await editor
					.getByText('Managed config file path must stay within its target directory', {
						exact: true
					})
					.waitFor();
				await editor.getByRole('button', { name: 'Reset', exact: true }).click();
				assert.equal(
					await filesEditor.getByLabel('Config file 1 path', { exact: true }).inputValue(),
					'appsettings.json'
				);
				await filesEditor
					.getByRole('button', { name: 'Remove config file 2', exact: true })
					.click();
				await filesEditor
					.getByRole('button', { name: 'Remove config file 1', exact: true })
					.click();
				await editor.getByRole('button', { name: 'Save candidate', exact: true }).click();
				await editor.getByText('Unsaved edits', { exact: true }).waitFor({ state: 'hidden' });
				assert.deepEqual(managedDrafts.get('v2').config_files, []);
				await page.reload();
				await filesEditor
					.getByText('No managed config files will be included in this candidate.', { exact: true })
					.waitFor();
				await editor.getByRole('button', { name: 'Next release', exact: true }).click();
				await filesEditor
					.getByText('No managed config files will be included in this candidate.', { exact: true })
					.waitFor();
				await pendingPill.click();
				await filesEditor.getByRole('button', { name: 'Add config file', exact: true }).click();
				await filesEditor.getByLabel('Config file 1 path', { exact: true }).fill('web.config');
				await filesEditor
					.getByLabel('Config file 1 target', { exact: true })
					.selectOption('release_dir');
				await filesEditor
					.getByLabel('Config file 1 contents', { exact: true })
					.fill('<approved />');
				const approvalsBeforeFiles = approvals;
				await editor.getByRole('button', { name: 'Approve & deploy', exact: true }).click();
				await editor
					.getByText('Staged artifact checksum changed; download again', { exact: true })
					.waitFor();
				assert.equal(approvals, approvalsBeforeFiles + 1);
				assert.deepEqual(candidateWrites.at(-1).services[0].config_files, [
					{ file_path: 'web.config', target: 'release_dir', content: '<approved />' }
				]);
				assert.equal(liveConfigWrites, 0);
				assert.equal(activeFiles[0].content, '{"active":true}');
				assert.deepEqual(errors, []);
				console.log(
					`PASS ${prefix || '/'} ${viewport.width}px: browse, host download, configure, failed deploy, review, discard, retained target, rollback review, active-deploy guards, failed loads, artifact-switch conflict, preserved edits, managed file staging/reload/removal/approval/validation; no page errors`
				);
				await page.close();
			}
	} finally {
		await browser?.close();
		await new Promise((resolve) => server.close(resolve));
	}
})().catch((error) => {
	console.error(error);
	process.exitCode = 1;
});
