import type { Service, ServiceType } from './api/types';
import { getAuthPassword, setAuthPassword, clearAuthPassword } from './api/transport';
import { openAuthenticatedEventStream } from './api/streams';

export * from './api/types';
export {
	APIRequestError,
	errorMessageFromResponse,
	getAuthPassword,
	setAuthPassword,
	clearAuthPassword
} from './api/transport';
export { openAuthenticatedEventStream, type AuthenticatedEventStream } from './api/streams';

export function isIISService(serviceType: Service['service_type'] | ServiceType): boolean {
	return serviceType === 'iis' || serviceType === 'static';
}

export function serviceTypeLabel(serviceType: Service['service_type'] | ServiceType): string {
	return isIISService(serviceType) ? 'IIS Site' : 'Binary (NSSM)';
}

export function iisAppKindLabel(kind: string): string {
	switch (kind) {
		case 'php':
			return 'PHP';
		case 'aspnet_classic':
			return 'ASP.NET Classic';
		default:
			return 'Static Site';
	}
}

// ── API methods ──────────────────────────────────────────────

export const auth = {
	getPassword: getAuthPassword,
	setPassword: setAuthPassword,
	clearPassword: clearAuthPassword,
	hasPassword: () => getAuthPassword() !== ''
};

import { releasesApi } from './api/releases';
import { systemApi } from './api/system';
import { watchersApi } from './api/watchers';
import { webhooksApi } from './api/webhooks';
import { streamsApi } from './api/streams-client';
import { servicesApi } from './api/services';
import { configurationApi } from './api/configuration';
export const api = {
	...releasesApi,
	...systemApi,
	...watchersApi,
	...webhooksApi,
	...streamsApi,
	...servicesApi,
	...configurationApi
};
