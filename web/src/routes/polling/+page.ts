import { api } from '$lib/api';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ depends }) => {
	depends('app:watchers');
	try {
		return { watchers: await api.listWatchers(), loadError: '' };
	} catch (error) {
		return {
			watchers: [],
			loadError: error instanceof Error ? error.message : 'Failed to load watchers for polling.'
		};
	}
};
