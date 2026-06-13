import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
	return twMerge(clsx(inputs));
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type WithoutChild<T> = T extends { child?: any } ? Omit<T, "child"> : T;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type WithoutChildren<T> = T extends { children?: any } ? Omit<T, "children"> : T;
export type WithoutChildrenOrChild<T> = WithoutChildren<WithoutChild<T>>;
export type WithElementRef<T, U extends HTMLElement = HTMLElement> = T & { ref?: U | null };

export function timeAgo(dateString: string | null | undefined): string {
	if (!dateString) return '—';
	const date = new Date(dateString);
	const now = new Date();
	const diffMs = now.getTime() - date.getTime();
	
	const diffSec = Math.floor(diffMs / 1000);
	if (diffSec < 60) return `${diffSec}s ago`;
	
	const diffMin = Math.floor(diffSec / 60);
	if (diffMin < 60) return `${diffMin}m ago`;
	
	const diffHour = Math.floor(diffMin / 60);
	if (diffHour < 24) return `${diffHour}h ago`;
	
	const diffDay = Math.floor(diffHour / 24);
	if (diffDay < 30) return `${diffDay}d ago`;
	
	return date.toLocaleDateString();
}

export function formatDate(ts: string | null): string {
	if (!ts) return '—';
	return new Date(ts).toLocaleString();
}

export function formatDuration(ms: number): string {
	if (!ms) return '—';
	if (ms < 1000) return `${ms}ms`;
	return `${(ms / 1000).toFixed(1)}s`;
}

export function statusColor(s: string): string {
	switch (s) {
		case 'healthy':
			return 'bg-emerald-500/15 text-emerald-400 border-emerald-500/30';
		case 'deploying':
			return 'bg-blue-500/15 text-blue-400 border-blue-500/30';
		case 'failed':
			return 'bg-red-500/15 text-red-400 border-red-500/30';
		case 'rollback':
			return 'bg-amber-500/15 text-amber-400 border-amber-500/30';
		default:
			return 'bg-muted text-muted-foreground border-border';
	}
}

export function compareSemver(a: string, b: string): number {
	const parse = (v: string): [number, number, number] => {
		const cleaned = (v || '').trim().replace(/^v/i, '');
		const parts = cleaned.split('.');
		const out: [number, number, number] = [0, 0, 0];
		for (let i = 0; i < 3 && i < parts.length; i++) {
			const n = Number.parseInt(parts[i], 10);
			out[i] = Number.isFinite(n) ? n : 0;
		}
		return out;
	};

	const pa = parse(a);
	const pb = parse(b);
	for (let i = 0; i < 3; i++) {
		if (pa[i] > pb[i]) return 1;
		if (pa[i] < pb[i]) return -1;
	}
	return 0;
}

export function healthBadgeColor(s: string): string {
	switch (s) {
		case 'healthy':
			return 'bg-emerald-500/15 text-emerald-400 border-emerald-500/30';
		case 'unhealthy':
			return 'bg-red-500/15 text-red-400 border-red-500/30';
		default:
			return 'bg-amber-500/15 text-amber-400 border-amber-500/30';
	}
}
