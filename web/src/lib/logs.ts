export type LogLevel = 'debug' | 'info' | 'warn' | 'error' | 'fatal' | 'unknown';
export interface LogEntry {
	id: number;
	time: string;
	level: LogLevel;
	message: string;
	component: string;
	fields: Record<string, unknown>;
	stack: string;
	raw: string;
}

function levelOf(value: unknown): LogLevel {
	if (typeof value === 'number') {
		if (value <= 8)
			return value < 0 ? 'debug' : value >= 8 ? 'error' : value >= 4 ? 'warn' : 'info';
		return value >= 60
			? 'fatal'
			: value >= 50
				? 'error'
				: value >= 40
					? 'warn'
					: value >= 30
						? 'info'
						: 'debug';
	}
	const text = String(value ?? '').toLowerCase();
	if (text === 'warning' || text === 'warn') return 'warn';
	if (text === 'critical' || text === 'fatal' || text === 'panic') return 'fatal';
	if (text === 'error' || text === 'err') return 'error';
	if (text === 'debug' || text === 'trace') return 'debug';
	return text === 'info' || text === 'information' ? 'info' : 'unknown';
}

export function fieldText(value: unknown): string {
	return typeof value === 'string' ? value : (JSON.stringify(value, null, 2) ?? String(value));
}

function stringField(value: unknown): string {
	return typeof value === 'string' ? value : '';
}

function parseLine(raw: string, id: number): LogEntry {
	const entry: LogEntry = {
		id,
		time: '',
		level: 'unknown',
		message: raw,
		component: '',
		fields: {},
		stack: '',
		raw
	};
	try {
		const parsed: unknown = JSON.parse(raw);
		if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
			const data = parsed as Record<string, unknown>;
			entry.message = stringField(data.msg) || stringField(data.message) || raw;
			const time = data.time ?? data.timestamp ?? data.ts;
			entry.time =
				typeof time === 'number'
					? new Date(time > 1e11 ? time : time * 1000).toISOString()
					: stringField(time);
			entry.level = levelOf(data.level ?? data.severity);
			entry.component = stringField(data.component) || stringField(data.logger);
			const standard = new Set([
				'msg',
				'message',
				'time',
				'timestamp',
				'ts',
				'level',
				'severity',
				'component',
				'logger'
			]);
			for (const [key, value] of Object.entries(data)) {
				if (standard.has(key)) continue;
				if (/^(stack|stacktrace|stack_trace|traceback)$/i.test(key))
					entry.stack += (entry.stack ? '\n' : '') + fieldText(value);
				else entry.fields[key] = value;
			}
			const error = data.error ?? data.err ?? data.exception;
			if (error && typeof error === 'object' && !Array.isArray(error)) {
				const detail = error as Record<string, unknown>;
				const stack = detail.stack ?? detail.stacktrace ?? detail.stack_trace;
				if (stack && !entry.stack) entry.stack = fieldText(stack);
			} else if (typeof error === 'string' && error.includes('\n') && !entry.stack) {
				entry.stack = error;
			}
			return entry;
		}
	} catch {
		/* Plain output and partial JSON remain readable verbatim. */
	}
	const prefix = /^(?:\[([^\]]+)\]|(\d{4}[-/]\d{2}[-/]\d{2}[T ][\d:.+Z -]+))\s*/.exec(raw);
	if (prefix) {
		entry.time = prefix[1] || prefix[2];
		entry.message = raw.slice(prefix[0].length);
	}
	const severity = /^(?:\[)?(DEBUG|INFO|WARN(?:ING)?|ERROR|FATAL|PANIC)(?:\])?[:\s]+/i.exec(
		entry.message
	);
	if (severity) {
		entry.level = levelOf(severity[1]);
		entry.message = entry.message.slice(severity[0].length);
	}
	if (/^(panic:|fatal error:)/i.test(raw)) entry.level = 'fatal';
	if (/^(Traceback|Unhandled exception)/i.test(raw)) entry.level = 'error';
	return entry;
}

// Keep stack frames attached to their event; never discard unparseable output.
export function parseLogLines(lines: string[]): LogEntry[] {
	const entries: LogEntry[] = [];
	for (let i = 0; i < lines.length; i++) {
		// ANSI escape codes are expected in redirected console output.
		// eslint-disable-next-line no-control-regex
		const raw = lines[i].replace(/\u001b\[[0-9;]*m/g, '').replace(/^\uFEFF/, '');
		const previous = entries.at(-1);
		const frame =
			!/^\s*\{/.test(raw) && /^(\s+\S|goroutine \d+|created by |\s*at\s|\s*File ")/.test(raw);
		const panicContinuation =
			previous?.level === 'fatal' &&
			!/^\s*\{|^\d{4}[-/]\d{2}|^\[|^(DEBUG|INFO|WARN(?:ING)?|ERROR|FATAL|PANIC)[:\s]/i.test(raw);
		if (previous && (frame || panicContinuation || raw === '')) {
			previous.raw += '\n' + raw;
			if (raw) previous.stack += (previous.stack ? '\n' : '') + raw;
			continue;
		}
		if (raw) entries.push(parseLine(raw, i));
	}
	return entries;
}

export function logTime(value: string): string {
	if (!value) return '—';
	const date = new Date(value);
	return Number.isNaN(date.getTime())
		? value
		: date.toLocaleTimeString(undefined, {
				hour12: false,
				hour: '2-digit',
				minute: '2-digit',
				second: '2-digit'
			});
}
