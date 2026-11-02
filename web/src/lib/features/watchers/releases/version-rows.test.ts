import { describe, it as test } from 'node:test';
import assert from 'node:assert/strict';
import type { CatalogDownload, ReleaseInfo, ReleaseMark } from '$lib/api';
import { buildVersionRows } from './version-rows';

function retained(version: string, mod_time: string, is_current = false): ReleaseInfo {
	return { version, mod_time, is_current, size_bytes: 1, size_human: '1 B', has_snapshot: true };
}

function download(id: number, version: string, downloaded_at: string): CatalogDownload {
	return {
		id,
		watcher_id: 1,
		release_id: id,
		asset_id: id,
		tag: version,
		version,
		asset_name: `${version}.zip`,
		sha256: '',
		downloaded_at
	};
}

function mark(version: string, blocked_at: string | null): ReleaseMark {
	return {
		id: 1,
		watcher_id: 1,
		version,
		tag: version,
		title: '',
		notes: '',
		url: '',
		published_at: null,
		blocked_at,
		block_reason: blocked_at ? 'bad' : ''
	};
}

describe('version rows', () => {
	test('merges a retained and downloaded version into one row, current first', () => {
		const rows = buildVersionRows({
			versions: [
				retained('v1', '2026-10-01T00:00:00Z'),
				retained('v2', '2026-10-02T00:00:00Z', true)
			],
			downloads: [
				download(1, 'v1', '2026-09-30T00:00:00Z'),
				download(2, 'v3', '2026-10-05T00:00:00Z')
			],
			marks: [],
			draftVersions: ['v1', 'v3'],
			currentVersion: 'v2'
		});
		assert.deepEqual(
			rows.map((row) => row.version),
			['v2', 'v3', 'v1']
		);
		const v1 = rows.find((row) => row.version === 'v1')!;
		assert.ok(v1.retained && v1.download);
		assert.equal(v1.hasDraft, true);
		// Drafts only apply to rollback, which needs a retained release.
		assert.equal(rows.find((row) => row.version === 'v3')!.hasDraft, false);
	});

	test('a block adds a version this host has never seen; notes alone do not', () => {
		const rows = buildVersionRows({
			versions: [retained('v1', '2026-10-01T00:00:00Z')],
			downloads: [],
			marks: [mark('v9', '2026-10-03T00:00:00Z'), mark('v8', null), mark('v1', null)],
			draftVersions: [],
			currentVersion: ''
		});
		assert.deepEqual(
			rows.map((row) => [row.version, row.blocked]),
			[
				['v9', true],
				['v1', false]
			]
		);
		assert.equal(rows[1].mark?.version, 'v1');
	});

	test('keeps the newest download of a version', () => {
		const rows = buildVersionRows({
			versions: [],
			downloads: [
				download(4, 'v2', '2026-10-01T00:00:00Z'),
				download(7, 'v2', '2026-10-02T00:00:00Z')
			],
			marks: [],
			draftVersions: [],
			currentVersion: ''
		});
		assert.equal(rows.length, 1);
		assert.equal(rows[0].download?.id, 7);
	});
});
