import { describe, it as test } from 'node:test';
import assert from 'node:assert/strict';
import type { ReleaseMark } from '$lib/api';
import { isBlocked, markFor, safeReleaseLink } from './release-marks';

function mark(overrides: Partial<ReleaseMark>): ReleaseMark {
	return {
		id: 1,
		watcher_id: 1,
		version: 'v1',
		tag: '',
		title: '',
		notes: '',
		url: '',
		published_at: null,
		blocked_at: null,
		block_reason: '',
		...overrides
	};
}

describe('release marks', () => {
	test('matches a catalog release by version or by tag', () => {
		const marks = [mark({ version: 'api-2.0.0', tag: 'v2' }), mark({ id: 2, version: 'v3' })];
		assert.equal(markFor(marks, 'v2', 'v2')?.version, 'api-2.0.0');
		assert.equal(markFor(marks, 'v3')?.id, 2);
		assert.equal(markFor(marks, 'v4', ''), undefined);
		assert.equal(markFor(undefined, 'v3'), undefined);
	});

	test('a mark is blocked only when it has a block time', () => {
		assert.equal(isBlocked(mark({ blocked_at: '2026-11-02T00:00:00Z' })), true);
		assert.equal(isBlocked(mark({})), false);
		assert.equal(isBlocked(undefined), false);
	});

	test('only http(s) release links are rendered', () => {
		assert.equal(
			safeReleaseLink('https://github.com/o/r/releases/tag/v2'),
			'https://github.com/o/r/releases/tag/v2'
		);
		assert.equal(safeReleaseLink('javascript:alert(1)'), '');
		assert.equal(safeReleaseLink('not a url'), '');
		assert.equal(safeReleaseLink(''), '');
	});
});
