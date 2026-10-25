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
	page: number;
	has_next: boolean;
}
