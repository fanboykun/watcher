import { api } from '$lib/api';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ depends }) => {
	depends('app:watchers');
	const watchers = await api.listWatchers();
	return { watchers };
};
