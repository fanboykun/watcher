import type { CatalogDownload, ReleaseInfo, ReleaseMark } from '$lib/api';
import { isBlocked } from './release-marks';

/**
 * One version as this host knows it. A version can be retained on disk (it was
 * deployed), staged as a catalog download, blocked before it ever arrived, or several.
 */
export interface VersionRow {
	version: string;
	retained?: ReleaseInfo;
	/** Newest downloaded artifact for this version. */
	download?: CatalogDownload;
	mark?: ReleaseMark;
	current: boolean;
	blocked: boolean;
	hasDraft: boolean;
	/** Most recent time Watcher touched this version, for ordering. */
	seenAt: string;
}

export function buildVersionRows(input: {
	versions: ReleaseInfo[];
	downloads: CatalogDownload[];
	marks: ReleaseMark[];
	draftVersions: string[];
	currentVersion: string;
}): VersionRow[] {
	const rows = new Map<string, VersionRow>();
	const row = (version: string) => {
		let found = rows.get(version);
		if (!found) {
			found = { version, current: false, blocked: false, hasDraft: false, seenAt: '' };
			rows.set(version, found);
		}
		return found;
	};
	for (const retained of input.versions) {
		const found = row(retained.version);
		found.retained = retained;
		found.seenAt = later(found.seenAt, retained.mod_time);
	}
	for (const download of input.downloads) {
		const found = row(download.version);
		if (!found.download || download.id > found.download.id) found.download = download;
		found.seenAt = later(found.seenAt, download.downloaded_at);
	}
	for (const mark of input.marks) {
		// Notes alone do not make a version local; a block does, so it can be lifted here.
		const found = rows.get(mark.version) ?? (isBlocked(mark) ? row(mark.version) : undefined);
		if (!found) continue;
		found.mark = mark;
		if (!found.retained && !found.download)
			found.seenAt = later(found.seenAt, mark.blocked_at ?? '');
	}
	const drafts = new Set(input.draftVersions);
	const current = input.currentVersion.trim();
	for (const found of rows.values()) {
		found.current = found.version === current || Boolean(found.retained?.is_current);
		found.blocked = isBlocked(found.mark);
		found.hasDraft = drafts.has(found.version) && Boolean(found.retained);
	}
	return [...rows.values()].sort(
		(a, b) => Number(b.current) - Number(a.current) || timeOf(b.seenAt) - timeOf(a.seenAt)
	);
}

function later(a: string, b: string): string {
	return timeOf(b) > timeOf(a) ? b : a;
}

function timeOf(value: string): number {
	const time = Date.parse(value);
	return Number.isNaN(time) ? 0 : time;
}
