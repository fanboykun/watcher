import { isIISService, type IISAppKind, type WatcherWritePayload } from '$lib/api';
import type { createWatcherSetup } from './setup-controller.svelte';
import { feedAuthTypeFor, takesFeedCredentials } from '$lib/features/watchers/presentation';

export type WatcherSetupState = ReturnType<typeof createWatcherSetup>['ui'];

/** Builds the create-watcher request from the wizard state. */
export function buildCreatePayload(ui: WatcherSetupState): WatcherWritePayload {
	const github = ui.sourceType === 'github';
	return {
		name: ui.watcherName.trim(),
		service_name: ui.watcherServiceName.trim(),
		source_type: ui.sourceType,
		metadata_url: ui.metadataURL.trim(),
		...(takesFeedCredentials(ui.sourceType) && {
			feed_auth_type: feedAuthTypeFor(ui.sourceType, ui.feedAuth.type),
			feed_auth_username: ui.feedAuth.username.trim(),
			feed_auth_header: ui.feedAuth.header.trim(),
			feed_auth_region: ui.feedAuth.region.trim(),
			feed_auth_secret: ui.feedAuth.secret.trim()
		}),
		release_ref: ui.releaseRef.trim() || 'latest',
		deployment_environment: github ? ui.deploymentEnvironment.trim() : '',
		github_token: github && ui.useWatcherGitHubToken ? ui.watcherGitHubToken.trim() : '',
		check_interval_sec: ui.checkInterval,
		auto_deploy: ui.autoDeploy,
		install_dir: ui.installDir.trim(),
		hc_enabled: ui.healthChecksEnabled,
		hc_url: ui.healthCheckURL.trim(),
		webhook_enabled: ui.webhookEnabled,
		webhook_url: ui.webhookURL.trim(),
		webhook_signing_secret: ui.useCustomWebhookSigningSecret ? ui.webhookSigningSecret.trim() : '',
		notify_version_found: ui.webhookSelections.notify_version_found,
		notify_deployment_succeeded: ui.webhookSelections.notify_deployment_succeeded,
		notify_deployment_failed: ui.webhookSelections.notify_deployment_failed,
		notify_rollback_succeeded: ui.webhookSelections.notify_rollback_succeeded,
		notify_rollback_failed: ui.webhookSelections.notify_rollback_failed,
		notify_service_health_changed: ui.webhookSelections.notify_service_health_changed,
		services: ui.serviceDrafts.map((svc) => ({
			service_type: isIISService(svc.service_type || 'nssm') ? 'iis' : 'nssm',
			windows_service_name: (svc.windows_service_name || '').trim(),
			binary_name: (svc.binary_name || '').trim(),
			start_arguments: (svc.start_arguments || '').trim(),
			env_file: (svc.env_file || '').trim(),
			health_check_url: (svc.health_check_url || '').trim(),
			iis_app_kind: (svc.iis_app_kind || 'static') as IISAppKind,
			iis_app_pool: (svc.iis_app_pool || '').trim(),
			iis_site_name: (svc.iis_site_name || '').trim(),
			iis_managed_runtime: (svc.iis_managed_runtime || '').trim(),
			public_url: (svc.public_url || '').trim(),
			env_content: svc.env_content || '',
			config_files: (svc.config_files || []).map((file) => ({
				file_path: file.file_path.trim(),
				target: file.target || 'app_dir',
				content: file.content || ''
			}))
		}))
	};
}
