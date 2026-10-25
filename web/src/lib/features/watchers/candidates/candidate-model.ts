import type { Watcher } from '$lib/api';
import type { ServiceConfigFile, ServiceCandidateInfo } from '$lib/api';
import { copyFiles } from '$lib/features/configuration/model';
export interface EditableServiceCandidate {
	serviceId: number;
	serviceName: string;
	serviceType: string;
	activeEnv: string;
	candidateEnv: string;
	originalCandidateEnv: string;
	hasCandidateEnv: boolean;
	revisionTarget: string;
	activeFiles: ServiceConfigFile[];
	candidateFiles: ServiceConfigFile[];
	originalCandidateFiles: ServiceConfigFile[];
	filesExplicit: boolean;
	originalFilesExplicit: boolean;
}
export function filesEqual(a: ServiceConfigFile[], b: ServiceConfigFile[]) {
	return JSON.stringify(copyFiles(a)) === JSON.stringify(copyFiles(b));
}
export function serviceChanged(service: EditableServiceCandidate) {
	return (
		service.candidateEnv !== service.originalCandidateEnv ||
		service.filesExplicit !== service.originalFilesExplicit ||
		!filesEqual(service.candidateFiles, service.originalCandidateFiles)
	);
}
export function resetCandidate(service: EditableServiceCandidate) {
	service.candidateEnv = service.originalCandidateEnv;
	service.candidateFiles = copyFiles(service.originalCandidateFiles);
	service.filesExplicit = service.originalFilesExplicit;
}
export function buildCandidatePayload(services: EditableServiceCandidate[]) {
	return services.map((service) => ({
		service_id: service.serviceId,
		env_content: service.candidateEnv,
		...(service.filesExplicit ? { config_files: copyFiles(service.candidateFiles) } : {})
	}));
}
export function editableCandidate(
	s: ServiceCandidateInfo,
	target: string
): EditableServiceCandidate {
	return {
		serviceId: s.service_id,
		serviceName: s.service_name,
		serviceType: s.service_type,
		activeEnv: s.active_env,
		candidateEnv: s.candidate_env,
		originalCandidateEnv: s.candidate_env,
		hasCandidateEnv: s.has_candidate_env,
		revisionTarget: s.candidate_target ?? (s.has_candidate_env ? target : ''),
		activeFiles: copyFiles(s.active_config_files),
		candidateFiles: copyFiles(s.candidate_config_files ?? s.active_config_files),
		originalCandidateFiles: copyFiles(s.candidate_config_files ?? s.active_config_files),
		filesExplicit: s.has_candidate_config_files ?? false,
		originalFilesExplicit: s.has_candidate_config_files ?? false
	};
}
export function markSaved(services: EditableServiceCandidate[], target: string) {
	for (const service of services) {
		service.originalCandidateEnv = service.candidateEnv;
		service.originalCandidateFiles = copyFiles(service.candidateFiles);
		service.originalFilesExplicit = service.filesExplicit;
		service.hasCandidateEnv = true;
		service.revisionTarget = target;
	}
}

export type CandidateProps = {
	watcher: Watcher;
	busy?: boolean;
	onApprove?: (version: string, catalogId: number) => Promise<void>;
	onDiscard?: () => Promise<void>;
	onRefreshWatcher?: () => Promise<void>;
};
