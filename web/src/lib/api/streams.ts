import { API_BASE, authHeader } from './transport';

export interface AuthenticatedEventStream {
	close(): void;
}

type EventStreamHandlers = {
	onMessage: (data: string) => void | Promise<void>;
	onError?: (error: unknown) => void;
};

function eventData(block: string): string | null {
	const lines = block.split(/\r?\n/);
	const data = lines
		.filter((line) => line.startsWith('data:'))
		.map((line) => line.slice(5).trimStart());
	return data.length > 0 ? data.join('\n') : null;
}

function delay(ms: number) {
	return new Promise((resolveDelay) => setTimeout(resolveDelay, ms));
}

export function openAuthenticatedEventStream(
	path: string,
	handlers: EventStreamHandlers,
	options: { reconnect?: boolean } = {}
): AuthenticatedEventStream {
	const controller = new AbortController();
	let closed = false;

	const run = async () => {
		do {
			try {
				const res = await fetch(`${API_BASE}${path}`, {
					headers: authHeader(),
					signal: controller.signal
				});
				if (!res.ok) {
					throw new Error(res.statusText || `HTTP ${res.status}`);
				}
				if (!res.body) {
					throw new Error('stream body is not available');
				}

				const reader = res.body.getReader();
				const decoder = new TextDecoder();
				let buffer = '';
				while (!closed) {
					const { done, value } = await reader.read();
					if (done) break;
					buffer += decoder.decode(value, { stream: true });

					let boundary = buffer.indexOf('\n\n');
					while (boundary >= 0) {
						const block = buffer.slice(0, boundary);
						buffer = buffer.slice(boundary + 2);
						const data = eventData(block);
						if (data !== null) {
							await handlers.onMessage(data);
						}
						boundary = buffer.indexOf('\n\n');
					}
				}
			} catch (error) {
				if (!closed) {
					handlers.onError?.(error);
				}
			}

			if (!closed && options.reconnect) {
				await delay(1500);
			}
		} while (!closed && options.reconnect);
	};

	void run();

	return {
		close() {
			closed = true;
			controller.abort();
		}
	};
}
