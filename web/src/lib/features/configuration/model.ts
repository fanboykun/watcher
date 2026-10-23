import type { ServiceConfigFile } from '$lib/api';

export function copyFiles(files: ServiceConfigFile[] = []): ServiceConfigFile[] {
	return files.map(({ file_path, target, content }) => ({ file_path, target, content }));
}

export function configFingerprint(env: string, files: ServiceConfigFile[]): string {
	return JSON.stringify([env, copyFiles(files)]);
}

export function fileLocation(file: Pick<ServiceConfigFile, 'file_path' | 'target'>): string {
	return `${file.target}/${file.file_path.replaceAll('\\', '/').toLowerCase()}`;
}

export function configurationProblems(envPath: string, files: ServiceConfigFile[]): string[] {
	const problems: string[] = [];
	const locations = new Set<string>();
	if (envPath.trim()) locations.add(fileLocation({ target: 'app_dir', file_path: envPath.trim() }));
	for (const [index, file] of files.entries()) {
		const path = file.file_path.trim();
		if (
			!path ||
			/^[a-z]:|^[\\/]/i.test(path) ||
			path.split(/[\\/]/).some((part) => part === '..' || part === '.') ||
			/[<>:"|?*]/.test(path) ||
			Array.from(path).some((char) => char.charCodeAt(0) < 32)
		) {
			problems.push(`File ${index + 1}: enter a relative path inside its target directory.`);
			continue;
		}
		const location = fileLocation({ ...file, file_path: path });
		if (locations.has(location))
			problems.push(`File ${index + 1}: ${path} duplicates another file at this destination.`);
		locations.add(location);
	}
	return problems;
}

export interface EnvDiagnostic {
	from: number;
	to: number;
	severity: 'warning';
	message: string;
}

/** Advisory only: never parse and serialize the user's environment file. */
export function environmentDiagnostics(text: string): EnvDiagnostic[] {
	const diagnostics: EnvDiagnostic[] = [];
	const keys = new Set<string>();
	let offset = 0;
	let quote = '';
	for (const rawLine of text.split('\n')) {
		const start = offset;
		offset += rawLine.length + 1;
		const line = rawLine.endsWith('\r') ? rawLine.slice(0, -1) : rawLine;
		if (quote) {
			for (let i = 0; i < line.length; i++) {
				if (line[i] === '\\' && quote === '"') {
					i++;
					continue;
				}
				if (line[i] === quote) {
					quote = '';
					break;
				}
			}
			continue;
		}
		if (!line.trim() || line.trimStart().startsWith('#')) continue;
		const assignment = /^\s*(?:export\s+)?([A-Za-z_][\w]*)\s*=\s*(.*)$/.exec(line);
		if (!assignment) {
			diagnostics.push({
				from: start,
				to: start + line.length,
				severity: 'warning',
				message:
					'Unrecognized assignment. Raw content is preserved; check your application’s env-file syntax.'
			});
			continue;
		}
		const key = assignment[1];
		if (keys.has(key))
			diagnostics.push({
				from: start,
				to: start + line.length,
				severity: 'warning',
				message: `Duplicate key ${key}. Different env loaders may resolve it differently.`
			});
		keys.add(key);
		const value = assignment[2];
		if (value.startsWith('"') || value.startsWith("'")) {
			quote = value[0];
			for (let i = 1; i < value.length; i++) {
				if (value[i] === '\\' && quote === '"') {
					i++;
					continue;
				}
				if (value[i] === quote) {
					quote = '';
					break;
				}
			}
		}
	}
	if (quote)
		diagnostics.push({
			from: Math.max(0, text.lastIndexOf('\n') + 1),
			to: text.length,
			severity: 'warning',
			message: 'Unclosed quoted value. Check your application’s env-file syntax.'
		});
	return diagnostics;
}
