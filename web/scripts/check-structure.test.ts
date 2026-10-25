import { it as test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { inspect } from './check-structure';

test('source cap accepts 350 lines and rejects 351, including blank lines and comments', async () => {
	const root = await mkdtemp(join(tmpdir(), 'watcher-source-cap-'));
	try {
		await mkdir(join(root, 'feature'));
		await writeFile(join(root, 'valid.ts'), '// source\n'.repeat(350));
		await writeFile(join(root, 'feature', 'oversized.svelte'), '\r\n'.repeat(351));
		assert.deepEqual(await inspect(root), ['feature/oversized.svelte: 351 lines (maximum 350)']);
	} finally {
		await rm(root, { recursive: true, force: true });
	}
});
