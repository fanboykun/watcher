import { api, type SelfUpdateCheckResponse } from '$lib/api';

const SELF_UPDATE_CACHE_KEY = 'watcher.self-update.cache';
const SELF_UPDATE_DISMISS_KEY = 'watcher.self-update.dismissed-version';
const SELF_UPDATE_EVENT = 'watcher:self-update-changed';
const SELF_UPDATE_TTL_MS = 30 * 60 * 1000;

type SelfUpdateCacheRecord = {
	checked_at: string;
	info: SelfUpdateCheckResponse;
};

export type SelfUpdateSnapshot = {
	info: SelfUpdateCheckResponse | null;
	checkedAt: string | null;
	dismissedVersion: string;
	shouldNotify: boolean;
};

function canUseStorage() {
	return typeof window !== 'undefined' && typeof localStorage !== 'undefined';
}

function normalizeVersion(version: string | null | undefined) {
	return (version ?? '').trim();
}

function emitSelfUpdateChanged() {
	if (typeof window === 'undefined') return;
	window.dispatchEvent(new CustomEvent(SELF_UPDATE_EVENT));
}

function readCacheRecord(): SelfUpdateCacheRecord | null {
	if (!canUseStorage()) return null;

	const raw = localStorage.getItem(SELF_UPDATE_CACHE_KEY);
	if (!raw) return null;

	try {
		const parsed = JSON.parse(raw) as Partial<SelfUpdateCacheRecord>;
		if (
			typeof parsed.checked_at !== 'string' ||
			!parsed.info ||
			typeof parsed.info !== 'object' ||
			typeof parsed.info.latest_version !== 'string'
		) {
			return null;
		}
		return {
			checked_at: parsed.checked_at,
			info: parsed.info as SelfUpdateCheckResponse
		};
	} catch {
		return null;
	}
}

function writeCacheRecord(info: SelfUpdateCheckResponse) {
	if (!canUseStorage()) return;
	const record: SelfUpdateCacheRecord = {
		checked_at: new Date().toISOString(),
		info
	};
	localStorage.setItem(SELF_UPDATE_CACHE_KEY, JSON.stringify(record));
	emitSelfUpdateChanged();
}

function readDismissedVersion() {
	if (!canUseStorage()) return '';
	return normalizeVersion(localStorage.getItem(SELF_UPDATE_DISMISS_KEY));
}

function isCacheFresh(record: SelfUpdateCacheRecord | null) {
	if (!record) return false;
	const checkedAt = Date.parse(record.checked_at);
	if (Number.isNaN(checkedAt)) return false;
	return Date.now() - checkedAt < SELF_UPDATE_TTL_MS;
}

function shouldNotify(info: SelfUpdateCheckResponse | null, dismissedVersion: string) {
	if (!info?.update_available) return false;
	return normalizeVersion(info.latest_version) !== dismissedVersion;
}

export function getSelfUpdateSnapshot(): SelfUpdateSnapshot {
	const record = readCacheRecord();
	const dismissedVersion = readDismissedVersion();
	return {
		info: record?.info ?? null,
		checkedAt: record?.checked_at ?? null,
		dismissedVersion,
		shouldNotify: shouldNotify(record?.info ?? null, dismissedVersion)
	};
}

export async function lookupSelfUpdate(options: { force?: boolean; silent?: boolean } = {}) {
	const { force = false, silent = false } = options;
	const record = readCacheRecord();
	if (!force && isCacheFresh(record)) {
		return record?.info ?? null;
	}

	try {
		const info = await api.selfUpdateCheck();
		writeCacheRecord(info);
		return info;
	} catch (error) {
		if (silent) {
			return record?.info ?? null;
		}
		throw error;
	}
}

export function dismissSelfUpdate(version: string) {
	if (!canUseStorage()) return;
	localStorage.setItem(SELF_UPDATE_DISMISS_KEY, normalizeVersion(version));
	emitSelfUpdateChanged();
}

export function clearSelfUpdateDismissal() {
	if (!canUseStorage()) return;
	localStorage.removeItem(SELF_UPDATE_DISMISS_KEY);
	emitSelfUpdateChanged();
}

export function clearSelfUpdateCache() {
	if (!canUseStorage()) return;
	localStorage.removeItem(SELF_UPDATE_CACHE_KEY);
	emitSelfUpdateChanged();
}

export function subscribeSelfUpdate(callback: (snapshot: SelfUpdateSnapshot) => void) {
	if (typeof window === 'undefined') {
		return () => {};
	}

	const handleChange = () => {
		callback(getSelfUpdateSnapshot());
	};

	window.addEventListener(SELF_UPDATE_EVENT, handleChange);
	window.addEventListener('storage', handleChange);

	return () => {
		window.removeEventListener(SELF_UPDATE_EVENT, handleChange);
		window.removeEventListener('storage', handleChange);
	};
}
