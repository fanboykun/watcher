import type { ServiceConfigFile } from './services';

export interface DeployLog {
	id: number;
	watcher_id: number;
	triggered_by: 'agent' | 'manual' | string;
	kind: 'deploy' | 'rollback' | string;
	reason: string;
	version: string;
	from_version: string;
	failed_target_version: string;
	status: string;
	error: string;
	failure_phase: string;
	duration_ms: number;
	github_deployment_id: number;
	parent_attempt_id?: number | null;
	root_attempt_id?: number | null;
	logs: string | null;
	started_at: string | null;
	completed_at: string | null;
}

export interface RollbackResponse {
	message: string;
	version: string;
	deploy_log_id: number;
	log_url: string;
}

export interface ReleaseInfo {
	version: string;
	mod_time: string;
	size_bytes: number;
	size_human: string;
	is_current: boolean;
	has_snapshot: boolean;
}

/** What Watcher knows about one version: its published notes, and whether it is blocked. */
export interface ReleaseMark {
	id: number;
	watcher_id: number;
	version: string;
	tag: string;
	title: string;
	notes: string;
	url: string;
	published_at: string | null;
	blocked_at: string | null;
	block_reason: string;
}

/** A retained version's saved configuration and the draft a manual rollback would apply. */
export interface RollbackConfigService {
	service_id: number;
	service_name: string;
	env_file: string;
	snapshot_env: string;
	snapshot_files: ServiceConfigFile[];
	has_draft: boolean;
	draft_env: string;
	/** Null keeps the saved managed files. */
	draft_files: ServiceConfigFile[] | null;
}

export interface RollbackConfig {
	version: string;
	source: string;
	captured_at: string;
	services: RollbackConfigService[];
}

export interface CatalogAsset {
	id: number;
	name: string;
	size: number;
	digest: string;
	browser_download_url: string;
}

export interface CatalogRelease {
	id: number;
	name: string;
	tag_name: string;
	body: string;
	html_url: string;
	draft: boolean;
	prerelease: boolean;
	published_at: string;
	assets: CatalogAsset[];
}

export interface CatalogDownload {
	id: number;
	watcher_id: number;
	release_id: number;
	asset_id: number;
	tag: string;
	version: string;
	asset_name: string;
	sha256: string;
	downloaded_at: string;
}

export interface ReleaseCatalogResponse {
	repository: string;
	releases: CatalogRelease[];
	downloads: CatalogDownload[];
	marks: ReleaseMark[];
	page: number;
	has_next: boolean;
}
