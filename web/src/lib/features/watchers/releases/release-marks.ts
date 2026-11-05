import type { ReleaseMark } from '$lib/api';

/**
 * Finds the mark for a version. A catalog release is matched by its tag too, because a
 * version.json release can publish a service version that differs from the tag.
 */
export function markFor(
	marks: ReleaseMark[] | undefined,
	version: string,
	tag = ''
): ReleaseMark | undefined {
	return marks?.find((mark) => mark.version === version || (tag !== '' && mark.tag === tag));
}

export function isBlocked(mark: ReleaseMark | undefined | null): boolean {
	return Boolean(mark?.blocked_at);
}

/** Release links come from the source; only http(s) links are ever rendered. */
export function safeReleaseLink(url: string | undefined): string {
	if (!url) return '';
	try {
		const parsed = new URL(url);
		return parsed.protocol === 'https:' || parsed.protocol === 'http:' ? parsed.href : '';
	} catch {
		return '';
	}
}
