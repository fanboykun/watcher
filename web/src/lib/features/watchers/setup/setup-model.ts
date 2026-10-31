import type { Service, IISAppKind } from '$lib/api';
type ServiceDraft = Partial<Service>;
export function repoNameFromURL(raw: string) {
	const cleaned = raw.split('/releases')[0].replace(/\/$/, '');
	const parts = cleaned.split('/');
	return parts[parts.length - 1] || 'my-app';
}
export function serviceTypeFromAppKind(appKind: string | undefined) {
	switch ((appKind || 'nssm').toLowerCase()) {
		case 'static':
		case 'php':
		case 'aspnet_classic':
			return 'iis';
		default:
			return 'nssm';
	}
}
export function iisKindFromAppKind(appKind: string | undefined): IISAppKind {
	switch ((appKind || 'static').toLowerCase()) {
		case 'php':
			return 'php';
		case 'aspnet_classic':
			return 'aspnet_classic';
		default:
			return 'static';
	}
}
export function defaultService(name: string, healthURL: string): ServiceDraft {
	const baseName = name;
	return {
		service_type: 'nssm',
		windows_service_name: baseName,
		binary_name: `${baseName}.exe`,
		start_arguments: '',
		env_file: '.env',
		env_content: '',
		iis_app_kind: 'static',
		iis_app_pool: '',
		iis_site_name: '',
		iis_managed_runtime: '',
		public_url: '',
		config_files: [],
		health_check_url: healthURL
	};
}

export function draftFromInspect(
	name: string,
	target: import('$lib/api').InspectServiceTarget | undefined,
	healthURL: string
): ServiceDraft | null {
	if (!target) return null;
	const serviceType = serviceTypeFromAppKind(target.app_kind);
	return {
		service_type: serviceType,
		windows_service_name: target.windows_service_name || name,
		binary_name: serviceType === 'nssm' ? target.binary_name || `${name}.exe` : '',
		start_arguments: target.start_arguments || '',
		env_file: serviceType === 'nssm' ? target.env_file || '.env' : '',
		env_content: '',
		iis_app_kind: iisKindFromAppKind(target.app_kind),
		iis_app_pool: target.iis_app_pool || target.windows_service_name || name,
		iis_site_name: target.iis_site_name || target.windows_service_name || name,
		iis_managed_runtime: target.iis_managed_runtime || '',
		public_url: target.public_url || '',
		config_files: [],
		health_check_url: target.health_check_url || healthURL
	};
}
