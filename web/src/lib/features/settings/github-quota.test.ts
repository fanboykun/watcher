import { describe, it as test } from 'node:test';
import assert from 'node:assert/strict';
import type { GitHubQuota } from '$lib/api';
import { quotaSourceLabel, quotaUsage, quotaView } from './github-quota';

const base: GitHubQuota = {
	source: 'global',
	watcher_ids: [1, 2],
	limit: 5000,
	remaining: 4200,
	reserve: 500,
	reset_at: '2026-10-31T12:00:00Z',
	blocked_until: null
};

describe('GitHub quota presentation', () => {
	test('a block outranks any reported quota', () => {
		const view = quotaView({ ...base, blocked_until: '2026-10-31T12:30:00Z' });
		assert.equal(view.tone, 'warn');
		assert.equal(view.label, 'Rate limited');
	});

	test('unknown quota is neutral until GitHub reports one', () => {
		assert.equal(quotaView({ ...base, limit: 0, reset_at: null }).tone, 'unknown');
	});

	test('the reserve is flagged as polling paused', () => {
		const view = quotaView({ ...base, remaining: 500 });
		assert.equal(view.tone, 'warn');
		assert.match(view.detail, /Polling waits/);
	});

	test('healthy quota shows what is left', () => {
		const view = quotaView(base);
		assert.equal(view.tone, 'ok');
		assert.match(view.label, /4,?200 of 5,?000 left/);
	});

	test('labels describe the token source and its users', () => {
		assert.equal(quotaSourceLabel({ ...base, source: 'anonymous' }), 'No token (shared per IP)');
		assert.equal(quotaUsage({ ...base, watcher_ids: [3] }), 'Used by 1 watcher');
		assert.match(quotaUsage({ ...base, watcher_ids: [] }), /self-update/);
	});
});
