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
