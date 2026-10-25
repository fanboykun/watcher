import type { Service, ServiceWritePayload, ServiceCandidateInfo } from './services';
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

export interface WatcherCandidateResponse {
	has_pending_release: boolean;
	target_version: string;
	status: string;
	intercept_next_release: boolean;
	auto_deploy: boolean;
	services: ServiceCandidateInfo[];
	saved_targets: string[];
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
