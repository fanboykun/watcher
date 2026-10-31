export function draftKey(ui: Record<string, unknown>) {
	return JSON.stringify([
		ui.cfgEnvironment,
		ui.cfgGithubDeployEnabled,
		ui.cfgLogDir,
		ui.cfgNssmPath,
		ui.cfgDBPath,
		ui.cfgAPIPort,
		ui.cfgAPIBaseURL,
		ui.cfgWebAssetsPath,
		ui.cfgWatcherRepoURL,
		ui.cfgWatcherServiceName,
		ui.cfgWebhookDefaultURL,
		ui.cfgWebhookTimeoutSec,
		ui.cfgWebhookRetryScheduleSec,
		ui.cfgWebhookAutoPauseEnabled,
		ui.cfgWebhookAutoPauseAfterFailures,
		ui.cfgWebhookEventRetentionDays,
		ui.cfgWebhookDeliveryRetentionDays,
		ui.githubTokenInput,
		ui.clearGitHubToken,
		ui.webhookDefaultSigningSecretInput,
		ui.clearWebhookDefaultSigningSecret
	]);
}
