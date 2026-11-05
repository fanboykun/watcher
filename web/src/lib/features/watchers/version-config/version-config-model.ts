import type { RollbackConfigService, ServiceConfigFile } from '$lib/api';
import { copyFiles } from '$lib/features/configuration/model';

/** One service's configuration for a retained version: what it ran with, and the edits to roll back with. */
export interface EditableVersionService {
	serviceId: number;
	serviceName: string;
	envPath: string;
	snapshotEnv: string;
	snapshotFiles: ServiceConfigFile[];
	/** The draft when one is saved, otherwise the snapshot. */
	savedEnv: string;
	savedFiles: ServiceConfigFile[];
	env: string;
	files: ServiceConfigFile[];
	hasDraft: boolean;
}

export function editableVersionService(service: RollbackConfigService): EditableVersionService {
	const savedEnv = service.has_draft ? service.draft_env : service.snapshot_env;
	const savedFiles = copyFiles(
		service.has_draft && service.draft_files ? service.draft_files : service.snapshot_files
	);
	return {
		serviceId: service.service_id,
		serviceName: service.service_name,
		envPath: service.env_file,
		snapshotEnv: service.snapshot_env,
		snapshotFiles: copyFiles(service.snapshot_files),
		savedEnv,
		savedFiles,
		env: savedEnv,
		files: copyFiles(savedFiles),
		hasDraft: service.has_draft
	};
}

function sameFiles(a: ServiceConfigFile[], b: ServiceConfigFile[]): boolean {
	return JSON.stringify(copyFiles(a)) === JSON.stringify(copyFiles(b));
}

/** Unsaved edits in the editor. */
export function isDirty(service: EditableVersionService): boolean {
	return service.env !== service.savedEnv || !sameFiles(service.files, service.savedFiles);
}

/** The editor's content differs from what the version last ran with. */
export function differsFromSnapshot(service: EditableVersionService): boolean {
	return service.env !== service.snapshotEnv || !sameFiles(service.files, service.snapshotFiles);
}

/**
 * Services whose edits must be saved as drafts, and services whose draft must be removed because
 * the editor is back to the snapshot. Drafts always carry the full file set, never "inherit active".
 */
export function draftChanges(services: EditableVersionService[]) {
	const save = services.filter(differsFromSnapshot).map((service) => ({
		service_id: service.serviceId,
		env_content: service.env,
		config_files: copyFiles(service.files)
	}));
	const discard = services
		.filter((service) => service.hasDraft && !differsFromSnapshot(service))
		.map((service) => service.serviceId);
	return { save, discard };
}

export function markSaved(services: EditableVersionService[]) {
	for (const service of services) {
		service.savedEnv = service.env;
		service.savedFiles = copyFiles(service.files);
		service.hasDraft = differsFromSnapshot(service);
	}
}
