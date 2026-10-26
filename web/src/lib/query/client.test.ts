import { describe, it as test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { QueryObserver } from '@tanstack/query-core';
import { createQueryClient, forget, invalidate, keepWithin, watcherScope } from './client';
import { keys } from './keys';

const readJSON = (path: string) => JSON.parse(readFileSync(new URL(path, import.meta.url), 'utf8'));

describe('server-state cache contracts', () => {
	test('deduplicates concurrent reads of the same resource', async () => {
		const client = createQueryClient();
		let calls = 0;
		let complete!: (value: string) => void;
		const queryFn = () => {
			calls++;
			return new Promise<string>((resolve) => {
				complete = resolve;
			});
		};
		const first = client.fetchQuery({ queryKey: keys.watcher(1), queryFn });
		const second = client.fetchQuery({ queryKey: keys.watcher(1), queryFn });
		complete('watcher');
		assert.deepEqual(await Promise.all([first, second]), ['watcher', 'watcher']);
		assert.equal(calls, 1);
		client.clear();
	});

	test('isolates resources, pagination, filters, versions and artifact identity', () => {
		const client = createQueryClient();
		const variants = [
			keys.candidate(1, 'v2', '1:7:v2'),
			keys.candidate(1, 'v2', '1:8:v2'),
			keys.candidate(1, 'next', '1:7:v2'),
			keys.candidate(2, 'v2', '2:7:v2'),
			keys.polls(1, 1, 10, 'all'),
			keys.polls(1, 2, 10, 'all'),
			keys.polls(1, 1, 25, 'all'),
			keys.polls(1, 1, 10, 'failed'),
			keys.snapshot(1, 'v1'),
			keys.snapshot(1, 'v2'),
			keys.logs(1, 100, 'agent', 'a'),
			keys.logs(1, 100, 'agent', 'b')
		];
		variants.forEach((key, index) => client.setQueryData(key, index));
		assert.deepEqual(
			variants.map((key) => client.getQueryData(key)),
			variants.map((_, index) => index)
		);
		client.clear();
	});

	test('invalidates the affected watcher without invalidating another watcher', async () => {
		const client = createQueryClient();
		const own = keys.candidate(1, 'v2', '1:7:v2'),
			other = keys.candidate(2, 'v2', '2:8:v2');
		client.setQueryData(own, 'draft');
		client.setQueryData(other, 'other');
		await invalidate(client, keys.watcher(1));
		assert.equal(client.getQueryState(own)?.isInvalidated, true);
		assert.equal(client.getQueryState(other)?.isInvalidated, false);
		client.clear();
	});

	test('cancels a read and clears private data without a late cache write', async () => {
		const client = createQueryClient();
		let signal!: AbortSignal;
		let finish!: (value: string) => void;
		const pending = client
			.fetchQuery({
				queryKey: keys.watcher(1),
				queryFn: (context) => {
					signal = context.signal;
					return new Promise<string>((resolve) => {
						finish = resolve;
					});
				}
			})
			.catch(() => undefined);
		await client.cancelQueries();
		client.clear();
		finish('old session');
		await pending;
		assert.equal(signal.aborted, true);
		assert.equal(client.getQueryData(keys.watcher(1)), undefined);
	});

	test('never retries an operational mutation automatically', async () => {
		const client = createQueryClient();
		let calls = 0;
		const operation = client.getMutationCache().build(client, {
			mutationFn: async () => {
				calls++;
				throw new Error('deployment rejected');
			}
		});
		await assert.rejects(operation.execute(undefined), /deployment rejected/);
		assert.equal(calls, 1);
		client.clear();
	});

	test('a watcher scope refreshes its list row and own tree but no other watcher', async () => {
		const client = createQueryClient();
		const touched = [keys.watchers, [...keys.watcher(1), 'detail'], keys.services, keys.status];
		const other = [...keys.watcher(2), 'detail'];
		[...touched, other].forEach((key) => client.setQueryData(key, 'cached'));
		await invalidate(client, ...watcherScope(1));
		touched.forEach((key) => assert.equal(client.getQueryState(key)?.isInvalidated, true));
		assert.equal(client.getQueryState(other)?.isInvalidated, false);
		client.clear();
	});

	test('forgets every cached read of a deleted resource', () => {
		const client = createQueryClient();
		client.setQueryData([...keys.service(4), 'detail'], 'deleted');
		client.setQueryData([...keys.service(4), 'logs', 'out', 100], 'deleted');
		client.setQueryData([...keys.service(5), 'detail'], 'kept');
		forget(client, keys.service(4));
		assert.equal(client.getQueryData([...keys.service(4), 'detail']), undefined);
		assert.equal(client.getQueryData([...keys.service(4), 'logs', 'out', 100]), undefined);
		assert.equal(client.getQueryData([...keys.service(5), 'detail']), 'kept');
		client.clear();
	});

	test('keeps previous rows across pages but never across resources or filters', () => {
		const client = createQueryClient();
		const options = (id: number, status: string, page: number) => ({
			queryKey: keys.polls(id, page, 10, status),
			queryFn: () => new Promise<string>(() => {}),
			placeholderData: keepWithin([...keys.watcher(id), 'polls', status])
		});
		client.setQueryData(keys.polls(1, 1, 10, 'all'), 'watcher 1 page 1');
		const observer = new QueryObserver(client, options(1, 'all', 1));
		const unsubscribe = observer.subscribe(() => {});
		observer.setOptions(options(1, 'all', 2));
		assert.equal(observer.getCurrentResult().data, 'watcher 1 page 1');
		assert.equal(observer.getCurrentResult().isPlaceholderData, true);
		observer.setOptions(options(1, 'failed', 1));
		assert.equal(observer.getCurrentResult().data, undefined);
		client.setQueryData(keys.polls(1, 1, 10, 'all'), 'watcher 1 page 1');
		observer.setOptions(options(1, 'all', 1));
		observer.setOptions(options(2, 'all', 2));
		assert.equal(observer.getCurrentResult().data, undefined);
		unsubscribe();
		client.clear();
	});

	test('uses the single query-core copy the Svelte adapter is built against', () => {
		const app = readJSON('../../../package.json');
		const adapter = readJSON('../../../node_modules/@tanstack/svelte-query/package.json');
		assert.equal(app.dependencies['@tanstack/svelte-query'], adapter.version);
		assert.equal(
			app.dependencies['@tanstack/query-core'],
			adapter.dependencies['@tanstack/query-core']
		);
	});
});
