import { openAuthenticatedEventStream } from './streams';
export const streamsApi = {
	streamWatcherEvents: (
		id: number,
		onMessage: (data: string) => void | Promise<void>,
		onError?: (error: unknown) => void
	) =>
		openAuthenticatedEventStream(
			`/watchers/${id}/events`,
			{ onMessage, onError },
			{ reconnect: true }
		),
	streamDeployLog: (
		id: number,
		logId: number,
		onMessage: (data: string) => void | Promise<void>,
		onError?: (error: unknown) => void
	) =>
		openAuthenticatedEventStream(`/watchers/${id}/deploys/${logId}/stream`, { onMessage, onError })
};
