export type AgentLogSource = 'agent' | 'stdout' | 'stderr';
export interface LogFileResponse {
	source: AgentLogSource | 'watcher';
	trace_id?: string;
	watcher_id?: number;
	log_file: string;
	format: 'json' | 'mixed';
	missing: boolean;
	lines: string[];
}

// ── Types ────────────────────────────────────────────────────

export interface AuthStatusResponse {
	authenticated: boolean;
	using_default_password: boolean;
}

export interface AuthBootstrapResponse {
	using_default_password: boolean;
	default_password?: string;
}

export interface InspectServiceTarget {
	name: string;
	version: string;
	artifact: string;
	artifact_url: string;
	published_at: string;
	app_kind: 'nssm' | 'static' | 'php' | 'aspnet_classic' | string;
	windows_service_name: string;
	binary_name: string;
	start_arguments: string;
	env_file: string;
	health_check_url: string;
	iis_app_pool: string;
	iis_site_name: string;
	iis_managed_runtime: string;
	public_url: string;
}

export interface InspectRepoResponse {
	source: 'manifest' | 'repo_assets' | string;
	repo_url: string;
	metadata_url: string;
	latest_version: string;
	published_at: string;
	assets: string[];
	services: Record<string, InspectServiceTarget>;
}

export interface Watcher {
	pending_catalog_id: number;
	polling_activity: 'active' | 'checking' | 'paused' | 'stopped' | 'unknown';
	poll_started_at: string | null;
	last_poll_status: string;
	last_poll_error: string;
	last_poll_id: string;
	last_poll_at: string | null;
	id: number;
	name: string;
	service_name: string;
	metadata_url: string;
	release_ref: string;
	deployment_environment: string;
	github_token?: string;
	has_github_token: boolean;
	github_token_masked: string;
	check_interval_sec: number;
	download_retries: number;
	install_dir: string;
	hc_enabled: boolean;
	hc_url: string;
	hc_retries: number;
	hc_interval_sec: number;
	hc_timeout_sec: number;
	paused: boolean;
	auto_deploy: boolean;
	intercept_next_release: boolean;
	pending_version: string;
	max_kept_versions: number;
	current_version: string;
	max_ignored_version: string;
	status: string;
	last_checked: string | null;
	last_deployed: string | null;
	last_error: string;
	webhook_enabled: boolean;
	webhook_url: string;
	has_webhook_signing_secret: boolean;
	webhook_signing_secret_masked: string;
	webhook_auto_pause_enabled_override?: boolean | null;
	webhook_auto_pause_after_failures_override?: number | null;
	webhook_paused_at: string | null;
	webhook_pause_reason: string;
	webhook_failure_streak: number;
	notify_version_found: boolean;
	notify_deployment_succeeded: boolean;
	notify_deployment_failed: boolean;
	notify_rollback_succeeded: boolean;
	notify_rollback_failed: boolean;
	notify_service_health_changed: boolean;
	services: Service[];
	created_at: string;
	updated_at: string;
}

export interface WatcherWritePayload {
	name?: string;
	service_name?: string;
	metadata_url?: string;
	release_ref?: string;
	deployment_environment?: string;
	github_token?: string;
	check_interval_sec?: number;
	download_retries?: number;
	install_dir?: string;
	hc_enabled?: boolean;
	hc_url?: string;
	hc_retries?: number;
	hc_interval_sec?: number;
	hc_timeout_sec?: number;
	paused?: boolean;
	auto_deploy?: boolean;
	max_kept_versions?: number;
	webhook_enabled?: boolean;
	webhook_url?: string;
	webhook_signing_secret?: string;
	webhook_auto_pause_enabled_override?: boolean | null;
	webhook_auto_pause_after_failures_override?: number | null;
	notify_version_found?: boolean;
	notify_deployment_succeeded?: boolean;
	notify_deployment_failed?: boolean;
	notify_rollback_succeeded?: boolean;
	notify_rollback_failed?: boolean;
	notify_service_health_changed?: boolean;
	services?: ServiceWritePayload[];
}

export type ServiceType = 'nssm' | 'iis';
export type IISAppKind = 'static' | 'php' | 'aspnet_classic';

export interface Service {
	id: number;
	watcher_id: number;
	service_type: ServiceType | 'static';
	windows_service_name: string;
	binary_name: string;
	start_arguments: string;
	env_file: string;
	health_check_url: string;
	iis_app_kind: IISAppKind;
	iis_app_pool: string;
	iis_site_name: string;
	iis_managed_runtime: string;
	public_url: string;
	env_content: string;
	last_service_status: string;
	last_service_error: string;
	last_service_checked_at?: string;
	last_health_status: string;
	last_health_http_status: number;
	last_health_error: string;
	last_health_checked_at?: string;
	config_files: ServiceConfigFile[];
	created_at: string;
	updated_at: string;
}

export interface ServiceWritePayload {
	service_type?: ServiceType | 'static';
	windows_service_name?: string;
	binary_name?: string;
	start_arguments?: string;
	env_file?: string;
	health_check_url?: string;
	iis_app_kind?: IISAppKind;
	iis_app_pool?: string;
	iis_site_name?: string;
	iis_managed_runtime?: string;
	public_url?: string;
	env_content?: string;
	config_files?: ServiceConfigFile[];
}

export type ConfigFileTarget = 'app_dir' | 'release_dir';

export interface ServiceConfigFile {
	id?: number;
	service_id?: number;
	file_path: string;
	target: ConfigFileTarget;
	content: string;
}

export interface ServiceWithWatcher extends Service {
	watcher_name: string;
	install_dir: string;
}

export interface ServiceConfigRevision {
	id: number;
	service_id: number;
	target_version: string;
	env_content: string;
	// null inherits active managed files; [] explicitly removes the managed file set.
	config_files: ServiceConfigFile[] | null;
	created_at: string;
	updated_at: string;
}

export interface ConfigSnapshotFile {
	file_path: string;
	target: 'env' | ConfigFileTarget;
	content: string;
}

export interface ConfigSnapshot {
	version: string;
	source: string;
	captured_at: string;
	services: { service_name: string; files: ConfigSnapshotFile[] }[];
}

export interface ServiceCandidateInfo {
	service_id: number;
	service_name: string;
	service_type: string;
	active_env: string;
	candidate_env: string;
	has_candidate_env: boolean;
	candidate_target: string;
	active_config_files: ServiceConfigFile[];
	candidate_config_files: ServiceConfigFile[];
	has_candidate_config_files: boolean;
	is_modified: boolean;
}

export interface WatcherCandidateResponse {
	has_pending_release: boolean;
	target_version: string;
	status: string;
	intercept_next_release: boolean;
	auto_deploy: boolean;
	services: ServiceCandidateInfo[];
	saved_targets: string[];
}

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

export interface HealthEvent {
	id: number;
	service_id: number;
	status: string;
	previous_status: string;
	source: string;
	http_status: number;
	error: string;
	checked_at: string | null;
}

export interface WebhookEvent {
	id: number;
	watcher_id: number;
	event_id: string;
	schema_version: string;
	event_type: string;
	dedupe_key: string;
	status: string;
	summary: string;
	payload: string;
	occurred_at: string;
	suppressed_at: string | null;
	last_delivery_at: string | null;
	created_at: string;
	updated_at: string;
}

export interface WebhookDelivery {
	id: number;
	watcher_id: number;
	webhook_event_id: number;
	event_id: string;
	event_type: string;
	summary: string;
	delivery_id: string;
	status: string;
	attempt_number: number;
	response_status_code: number;
	response_body: string;
	error: string;
	resolved_url: string;
	auth_type: string;
	secret_source: string;
	webhook_timestamp: number;
	webhook_signature: string;
	next_retry_at: string | null;
	last_attempt_at: string | null;
	completed_at: string | null;
	replayed_at: string | null;
	replayed_by: string;
	created_at: string;
	updated_at: string;
}

export interface WebhookDeliveryDetails {
	delivery: WebhookDelivery;
	event: {
		id: number;
		event_id: string;
		event_type: string;
		schema_version: string;
		status: string;
		summary: string;
		occurred_at: string;
		payload: unknown;
	};
	request: {
		url: string;
		auth_type: string;
		secret_source: string;
		headers: {
			content_type: string;
			webhook_id: string;
			webhook_timestamp: number;
			webhook_signature: string;
			x_watcher_event: string;
			x_watcher_delivery_id: string;
		};
	};
}

export interface PollEvent {
	poll_id: string;
	request_id: string;
	correlation_id: string;
	triggered_by: string;
	id: number;
	watcher_id: number;
	checked_at: string;
	status: string;
	remote_version: string;
	error: string;
}

export interface PaginatedResponse<T> {
	data: T[];
	total: number;
	page: number;
	pageSize: number;
}

export interface SystemStatus {
	status: string;
	version: string;
	uptime_seconds: number;
	uptime_human: string;
	watcher_count: number;
	service_count: number;
	deploys_24h: number;
}

export interface ReleaseInfo {
	version: string;
	mod_time: string;
	size_bytes: number;
	size_human: string;
	is_current: boolean;
	has_snapshot: boolean;
}

export interface SelfVersionResponse {
	version: string;
	go_version: string;
	os: string;
	arch: string;
	executable: string;
}

export interface SelfUpdateCheckResponse {
	update_available: boolean;
	current_version: string;
	latest_version: string;
	release_notes: string;
	download_url: string;
	published_at: string;
}

export interface SelfConfigResponse {
	environment: string;
	github_deploy_enabled: boolean;
	log_dir: string;
	nssm_path: string;
	db_path: string;
	api_port: string;
	api_base_url: string;
	watcher_repo_url: string;
	watcher_service_name: string;
	has_github_token: boolean;
	github_token_masked: string;
	webhook_default_url: string;
	has_webhook_default_signing_secret: boolean;
	webhook_default_signing_secret_masked: string;
	webhook_timeout_sec: number;
	webhook_retry_schedule_sec: string;
	webhook_auto_pause_enabled: boolean;
	webhook_auto_pause_after_failures: number;
	webhook_event_retention_days: number;
	webhook_delivery_retention_days: number;
	web_assets_path?: string;
	env_path: string;
}

export interface UpdateSelfConfigRequest {
	environment?: string;
	github_token?: string;
	github_deploy_enabled?: boolean;
	log_dir?: string;
	nssm_path?: string;
	db_path?: string;
	api_port?: string;
	api_base_url?: string;
	watcher_repo_url?: string;
	watcher_service_name?: string;
	webhook_default_url?: string;
	webhook_default_signing_secret?: string;
	webhook_timeout_sec?: number;
	webhook_retry_schedule_sec?: string;
	webhook_auto_pause_enabled?: boolean;
	webhook_auto_pause_after_failures?: number;
	webhook_event_retention_days?: number;
	webhook_delivery_retention_days?: number;
	web_assets_path?: string;
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
