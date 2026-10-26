export type ErrorKind =
	| 'not-found'
	| 'unauthorized'
	| 'forbidden'
	| 'offline'
	| 'timeout'
	| 'conflict'
	| 'invalid'
	| 'server'
	| 'unexpected';

export interface ErrorDescription {
	kind: ErrorKind;
	/** HTTP status when the failure came from Watcher or the router. */
	status: number | null;
	/** Short label shown as the screen's code, e.g. `404` or `OFFLINE`. */
	code: string;
	title: string;
	message: string;
	hint: string;
	requestId: string;
	/** Plain-text report for copying into an issue. */
	detail: string;
}

type APIErrorShape = Error & { status: number; responseBody?: unknown; requestId?: string };

const copy: Record<ErrorKind, { title: string; hint: string }> = {
	'not-found': {
		title: 'Nothing lives here',
		hint: 'It may have been deleted, or the link is out of date. Head back and pick it from the list.'
	},
	unauthorized: {
		title: 'Your session has ended',
		hint: 'Sign in again to keep working. Nothing on the agent was changed.'
	},
	forbidden: {
		title: 'Watcher refused this request',
		hint: 'Check the credentials and permissions used for this operation.'
	},
	offline: {
		title: 'Watcher is out of reach',
		hint: 'Make sure the agent service is running and reachable. After a restart or update it can take a few seconds to come back.'
	},
	timeout: {
		title: 'Watcher took too long to answer',
		hint: 'The agent may be busy with a deployment. Wait a moment, then try again.'
	},
	conflict: {
		title: 'Something changed in the meantime',
		hint: 'Another operation updated this resource. Reload it to see the current state.'
	},
	invalid: {
		title: 'Watcher rejected the request',
		hint: 'The address or values in this request are not valid.'
	},
	server: {
		title: 'Watcher hit an internal error',
		hint: 'Try again. If it keeps failing, the agent log usually explains why.'
	},
	unexpected: {
		title: 'This view failed to render',
		hint: 'This is a dashboard problem, not a deployment failure. Your services keep running. Reload to recover, and copy the details if you report it.'
	}
};

function isAPIError(error: unknown): error is APIErrorShape {
	return (
		error instanceof Error &&
		error.name === 'APIRequestError' &&
		typeof (error as Partial<APIErrorShape>).status === 'number'
	);
}

export function isUnauthorized(error: unknown): boolean {
	return isAPIError(error) && error.status === 401;
}

function kindFor(status: number | null, error: unknown): ErrorKind {
	if (status === null) return 'unexpected';
	if (status === 0) {
		const cause = isAPIError(error) ? error.responseBody : undefined;
		const timedOut =
			(cause instanceof Error && cause.name === 'TimeoutError') ||
			/timed out/i.test(error instanceof Error ? error.message : '');
		return timedOut ? 'timeout' : 'offline';
	}
	if (status === 401) return 'unauthorized';
	if (status === 403) return 'forbidden';
	if (status === 404 || status === 410) return 'not-found';
	if (status === 409) return 'conflict';
	if (status >= 500) return 'server';
	if (status >= 400) return 'invalid';
	return 'unexpected';
}

/** Watcher's own explanation, without the transport's generic guidance (the hint covers that). */
function serverDetail(error: APIErrorShape | null): string {
	const body = error?.responseBody;
	if (!body || typeof body !== 'object') return '';
	const value =
		(body as { error?: unknown; message?: unknown }).error ??
		(body as { message?: unknown }).message;
	if (typeof value !== 'string' || !value.trim() || /^\s*</.test(value)) return '';
	const text = value.trim();
	return `${text[0].toUpperCase()}${text.slice(1)}${/[.!?]$/.test(text) ? '' : '.'}`;
}

function rawMessage(error: unknown): string {
	if (error instanceof Error) return error.message;
	if (typeof error === 'string') return error;
	if (error && typeof error === 'object' && 'message' in error)
		return String((error as { message: unknown }).message ?? '');
	return '';
}

/**
 * Turns anything thrown or returned as an error into copy for a blocking screen.
 * `status` overrides the error's own status, as the router reports it separately.
 */
export function describeError(error: unknown, status?: number): ErrorDescription {
	const api = isAPIError(error) ? error : null;
	const resolved = status ?? api?.status ?? null;
	const kind = kindFor(resolved, error);
	const original = rawMessage(error).trim();
	const requestId = api?.requestId ?? /\(request_id: ([^)]+)\)/.exec(original)?.[1]?.trim() ?? '';
	const message = serverDetail(api) || original.replace(/\s*\(request_id: [^)]+\)/, '').trim();
	const code =
		kind === 'offline'
			? 'OFFLINE'
			: kind === 'timeout'
				? 'TIMEOUT'
				: resolved && resolved > 0
					? String(resolved)
					: 'ERROR';
	const stack = error instanceof Error && kind === 'unexpected' ? (error.stack ?? '') : '';
	const detail = [
		`kind: ${kind}`,
		resolved !== null ? `status: ${resolved}` : '',
		requestId ? `request_id: ${requestId}` : '',
		stack ||
			(error instanceof Error
				? `${error.name}: ${error.message}`
				: original && `message: ${original}`)
	]
		.filter(Boolean)
		.join('\n');
	return { kind, status: resolved, code, ...copy[kind], message, requestId, detail };
}
