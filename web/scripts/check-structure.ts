import { readdir, readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { relative, join } from 'node:path';

export const MAX_SOURCE_LINES = 350;
const root = fileURLToPath(new URL('../src', import.meta.url));
const extensions = /\.(?:svelte|[cm]?[jt]s|css)$/;

export function sourceLines(source: string) {
	const normalized = source.replace(/\r\n/g, '\n');
	return normalized === '' ? 0 : normalized.split('\n').length - Number(normalized.endsWith('\n'));
}

export async function inspect(directory: string, sourceRoot = directory): Promise<string[]> {
	const entries = await readdir(directory, { withFileTypes: true });
	const results = await Promise.all(
		entries.map(async (entry) => {
			const path = join(directory, entry.name);
			if (entry.isDirectory()) return inspect(path, sourceRoot);
			if (!extensions.test(entry.name)) return [];
			const count = sourceLines(await readFile(path, 'utf8'));
			return count > MAX_SOURCE_LINES
				? [`${relative(sourceRoot, path)}: ${count} lines (maximum ${MAX_SOURCE_LINES})`]
				: [];
		})
	);
	return results.flat();
}

if (import.meta.main) {
	const oversized = await inspect(root);
	if (oversized.length) {
		console.error(
			`Split oversized source files by responsibility before continuing:\n${oversized.sort().join('\n')}`
		);
		process.exitCode = 1;
	} else
		console.log(
			`Source structure passed: every application file is at most ${MAX_SOURCE_LINES} lines.`
		);
}
