import type { IISAppKind } from '$lib/api';
export type WizardStep = 1 | 2 | 3 | 4;
export const steps: Array<{ id: WizardStep; label: string; description: string }> = [
	{ id: 1, label: 'Repository', description: 'Connect your source' },
	{ id: 2, label: 'Watcher', description: 'Set delivery behavior' },
	{ id: 3, label: 'Services', description: 'Define what runs' },
	{ id: 4, label: 'Webhooks', description: 'Keep everyone informed' }
];
export const iisAppKinds: Array<{ value: IISAppKind; label: string; hint: string }> = [
	{
		value: 'static',
		label: 'Static Site',
		hint: 'HTML/CSS/JS or frontend build output served by IIS.'
	},
	{
		value: 'php',
		label: 'PHP',
		hint: 'PHP app hosted by IIS with FastCGI/PHP already installed.'
	},
	{
		value: 'aspnet_classic',
		label: 'ASP.NET Classic',
		hint: 'Classic ASP.NET app using the .NET CLR app pool.'
	}
];
