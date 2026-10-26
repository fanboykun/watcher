import { describe, it as test } from 'node:test';
import assert from 'node:assert/strict';
import { describeError, isUnauthorized } from './error-model';

function apiError(status: number, message: string, responseBody?: unknown) {
	return Object.assign(new Error(message), { name: 'APIRequestError', status, responseBody });
}

describe('blocking error descriptions', () => {
	test('classifies Watcher responses by status', () => {
		assert.equal(describeError(apiError(404, 'watcher not found')).kind, 'not-found');
		assert.equal(describeError(apiError(401, 'authentication required')).kind, 'unauthorized');
		assert.equal(describeError(apiError(409, 'deploy running')).kind, 'conflict');
		assert.equal(describeError(apiError(422, 'bad value')).kind, 'invalid');
		assert.equal(describeError(apiError(503, 'unavailable')).code, '503');
		assert.equal(describeError(apiError(500, 'boom')).kind, 'server');
	});

	test('separates unreachable agents from slow ones', () => {
		const offline = describeError(apiError(0, 'The dashboard could not reach Watcher.'));
		assert.equal(offline.kind, 'offline');
		assert.equal(offline.code, 'OFFLINE');
		const cause = Object.assign(new Error('signal timed out'), { name: 'TimeoutError' });
		assert.equal(describeError(apiError(0, 'Request failed', cause)).kind, 'timeout');
		assert.equal(describeError(apiError(0, 'Watcher details timed out.')).code, 'TIMEOUT');
	});

	test("shows Watcher's own explanation instead of generic guidance", () => {
		const info = describeError(
			apiError(404, 'watcher not found This item could not be found.', {
				error: 'watcher not found'
			})
		);
		assert.equal(info.message, 'Watcher not found.');
		const html = describeError(apiError(502, 'Watcher could not complete this request.', '<html>'));
		assert.equal(html.message, 'Watcher could not complete this request.');
	});

	test('moves the request id out of the visible message', () => {
		const info = describeError(apiError(500, 'Watcher failed. (request_id: abc-123)'));
		assert.equal(info.message, 'Watcher failed.');
		assert.equal(info.requestId, 'abc-123');
		assert.match(info.detail, /request_id: abc-123/);
	});

	test('treats render crashes and router errors distinctly', () => {
		const crash = describeError(
			new TypeError("Cannot read properties of undefined (reading 'id')")
		);
		assert.equal(crash.kind, 'unexpected');
		assert.match(crash.detail, /TypeError/);
		const route = describeError({ message: 'Not Found' }, 404);
		assert.equal(route.kind, 'not-found');
		assert.equal(route.code, '404');
	});

	test('only a 401 response ends the session', () => {
		assert.equal(isUnauthorized(apiError(401, 'authentication required')), true);
		assert.equal(isUnauthorized(apiError(0, 'unreachable')), false);
		assert.equal(isUnauthorized(apiError(500, 'boom')), false);
		assert.equal(isUnauthorized(new Error('401')), false);
	});
});
