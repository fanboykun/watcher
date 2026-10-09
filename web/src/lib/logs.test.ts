import { describe, it as test } from 'node:test';
import assert from 'node:assert/strict';
import { parseLogLines } from './logs';

describe('log processing', () => {
	test('decodes slog attributes and escaped stack traces', () => {
		const [entry] = parseLogLines([
			JSON.stringify({
				time: '2026-10-09T13:00:00Z',
				level: 'ERROR',
				msg: 'poll failed',
				component: 'alpha',
				watcher_id: 7,
				error: { message: 'timeout', stack: 'fetch()\n  main.go:42' }
			})
		]);
		assert.equal(entry.level, 'error');
		assert.equal(entry.message, 'poll failed');
		assert.equal(entry.fields.watcher_id, 7);
		assert.equal(entry.stack, 'fetch()\n  main.go:42');
	});
	test('keeps a Go panic and multiline stack as one event', () => {
		const entries = parseLogLines([
			'panic: runtime error',
			'',
			'goroutine 1 [running]:',
			'main.run()',
			'\tmain.go:42 +0x10',
			JSON.stringify({ level: 'INFO', msg: 'restarted' })
		]);
		assert.equal(entries.length, 2);
		assert.equal(entries[0].level, 'fatal');
		assert.ok(entries[0].stack.includes('main.go:42'));
		assert.equal(entries[1].message, 'restarted');
	});
	test('preserves non-JSON lines and broken JSON verbatim', () => {
		const lines = ['plain output', '{"msg":"unfinished', '[20:00:01] WARN: retrying'];
		const entries = parseLogLines(lines);
		assert.equal(entries.length, 3);
		assert.equal(entries[1].raw, lines[1]);
		assert.equal(entries[2].time, '20:00:01');
		assert.equal(entries[2].level, 'warn');
	});
	test('separates indented JSON records from stack continuations', () => {
		const entries = parseLogLines(['panic: failed', '  {"level":"INFO","msg":"next event"}']);
		assert.equal(entries.length, 2);
	});
	test('a severity-prefixed record starts a new event after a panic', () => {
		const entries = parseLogLines([
			'panic: failed',
			'main.run()',
			'\tmain.go:42',
			'INFO: restarted'
		]);
		assert.equal(entries.length, 2);
		assert.equal(entries[1].message, 'restarted');
	});
});
