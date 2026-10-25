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

export interface AuthStatusResponse {
	authenticated: boolean;
	using_default_password: boolean;
}

export interface AuthBootstrapResponse {
	using_default_password: boolean;
	default_password?: string;
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
