// API client for the watcher agent backend
import { base } from '$app/paths';
export const API_BASE = `${base}/api`;
const AUTH_PASSWORD_KEY = 'watcher.auth.password';

export function getAuthPassword(): string {
	if (typeof localStorage === 'undefined') return '';
	return localStorage.getItem(AUTH_PASSWORD_KEY) || '';
}

export function setAuthPassword(password: string) {
	if (typeof localStorage === 'undefined') return;
	localStorage.setItem(AUTH_PASSWORD_KEY, password);
}

export function clearAuthPassword() {
	if (typeof localStorage === 'undefined') return;
	localStorage.removeItem(AUTH_PASSWORD_KEY);
}

export function authHeader(): Record<string, string> {
	const password = getAuthPassword();
	return password ? { Authorization: `Bearer ${password}` } : {};
}

export class APIRequestError extends Error {
	constructor(
		message: string,
		public readonly status: number,
		public readonly responseBody?: unknown,
		public readonly requestId?: string
	) {
		super(message);
		this.name = 'APIRequestError';
	}
}

export function errorMessageFromResponse(
	status: number,
	statusText: string,
	body: unknown
): string {
	const detail =
		body && typeof body === 'object'
			? typeof (body as { error?: unknown }).error === 'string'
				? (body as { error: string }).error
				: typeof (body as { message?: unknown }).message === 'string'
					? (body as { message: string }).message
					: ''
			: typeof body === 'string'
				? body.trim()
				: '';
	const usable =
		detail &&
		!/^\s*</.test(detail) &&
		!/^(internal server error|server error|http \d+)$/i.test(detail);
	const guidance: Record<number, string> = {
		401: 'Your dashboard session expired or is no longer authorized. Sign in again, then retry.',
		403: 'This request was refused. Check the credentials and permissions for this operation.',
		404: 'This item could not be found. Refresh the page to check whether it still exists.',
		409: 'The current state changed or another operation is running. Refresh the details before trying again.',
		422: 'Check the entered values and submit again.',
		429: 'Too many requests. Wait before trying again.'
	};
	if (usable) return guidance[status] ? `${detail} ${guidance[status]}` : detail;
	if (guidance[status]) return guidance[status];
	if (status === 400) return 'Check the entered values and try again.';
	if (status >= 500)
		return 'Watcher could not complete this request. Try again; if it keeps failing, inspect the agent logs.';
	return statusText || `Request failed (HTTP ${status}).`;
}

export async function request<T>(path: string, options?: RequestInit): Promise<T> {
	const normalizedPath = path.startsWith('/api/') ? path.slice(4) : path;
	let res: Response;
	const timeout =
		!options?.method || options.method === 'GET' ? AbortSignal.timeout(30_000) : undefined;
	const signal =
		timeout && options?.signal
			? AbortSignal.any([timeout, options.signal])
			: (timeout ?? options?.signal);
	const clientRequestId = globalThis.crypto?.randomUUID?.();
	try {
		res = await fetch(`${API_BASE}${normalizedPath}`, {
			...options,
			signal,
			headers: {
				'Content-Type': 'application/json',
				...(clientRequestId ? { 'X-Request-ID': clientRequestId } : {}),
				...authHeader(),
				...options?.headers
			}
		});
	} catch (error) {
		if (timeout?.aborted && !options?.signal?.aborted)
			throw new APIRequestError(
				'Watcher details timed out. Check the agent connection and retry.',
				0,
				error
			);
		if (error instanceof Error && error.name === 'AbortError') throw error;
		throw new APIRequestError(
			'The dashboard could not reach Watcher. Check your connection and that the agent service is running. For a deployment action, refresh its history before retrying: the request may already have been accepted.',
			0,
			error
		);
	}
	if (!res.ok) {
		const raw = await res.text();
		let body: unknown = raw;
		try {
			body = raw ? JSON.parse(raw) : undefined;
		} catch {
			// Plain-text and HTML error bodies fall back to an actionable status message.
		}
		let message = errorMessageFromResponse(res.status, res.statusText, body);
		const requestId = res.headers.get('X-Request-ID') || undefined;
		if (requestId) message += ` (request_id: ${requestId})`;
		if (body && typeof body === 'object' && 'deploy_log_id' in body) {
			message += ` (deploy_log_id: ${String((body as { deploy_log_id?: unknown }).deploy_log_id ?? '')})`;
		}
		throw new APIRequestError(message, res.status, body, requestId);
	}
	if (res.status === 204) return undefined as T;
	try {
		return await res.json();
	} catch {
		throw new APIRequestError(
			'Watcher returned an unreadable response. Refresh the details to confirm the result before repeating the action.',
			res.status
		);
	}
}
