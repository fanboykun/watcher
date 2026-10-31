import type { ReleaseInfo, CatalogDownload, ReleaseCatalogResponse } from './types';
import { request } from './transport';
export const releasesApi = {
	getReleaseCatalog: (id: number, page = 1, signal?: AbortSignal) =>
		request<ReleaseCatalogResponse>(`/watchers/${id}/releases?page=${page}`, { signal }),
	downloadCatalogRelease: (id: number, releaseId: number, assetId: number) =>
		request<CatalogDownload>(`/watchers/${id}/releases/download`, {
			method: 'POST',
			body: JSON.stringify({ release_id: releaseId, asset_id: assetId })
		}),
	selectCatalogCandidate: (id: number, catalogId: number) =>
		request<{ message: string; version: string; catalog_id: number }>(
			`/watchers/${id}/releases/${catalogId}/candidate`,
			{ method: 'POST' }
		),
	deployCatalogRelease: (id: number, catalogId: number) =>
		request<{ message: string; deploy_log_id: number; version: string; catalog_id: number }>(
			`/watchers/${id}/releases/${catalogId}/deploy`,
			{ method: 'POST' }
		),
	watcherVersionCatalog: (id: number, signal?: AbortSignal) =>
		request<{ versions: ReleaseInfo[]; downloads: CatalogDownload[]; current_version: string }>(
			`/watchers/${id}/versions`,
			{ signal }
		)
};
