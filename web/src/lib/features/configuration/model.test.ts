import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import {
	configFingerprint,
	configurationProblems,
	copyFiles,
	environmentDiagnostics,
	fileLocation
} from './model';

describe('configuration documents', () => {
	it('compares raw text including comments, duplicate keys, quotes, and Windows line endings', () => {
		const text = '# deployment\r\nexport TOKEN="a=b # c"\r\nPORT=80\r\nPORT=81\r\n';
		assert.notEqual(
			configFingerprint(text, []),
			configFingerprint(text.replaceAll('\r\n', '\n'), [])
		);
		assert.notEqual(
			configFingerprint(text, []),
			configFingerprint(text.replace('# deployment', '# changed'), [])
		);
		assert.equal(
			environmentDiagnostics(text).filter((entry) => entry.message.startsWith('Duplicate key'))
				.length,
			1
		);
		assert.equal(text, '# deployment\r\nexport TOKEN="a=b # c"\r\nPORT=80\r\nPORT=81\r\n');
	});
	it('recognizes quoted multiline env values without treating their contents as assignments', () => {
		assert.deepEqual(
			environmentDiagnostics('CERT="first\nnot an assignment\nlast"\nPORT=80\n'),
			[]
		);
		assert.ok(
			environmentDiagnostics('TOKEN="unclosed').some((entry) => entry.message.includes('Unclosed'))
		);
	});
	it('treats diagnostics as advice and leaves unusual env content untouched', () => {
		const text = 'set PORT 80\n# keep this\nexport API_URL=https://example.test?a=b\n';
		assert.equal(environmentDiagnostics(text).length, 1);
		assert.equal(environmentDiagnostics(text)[0].from, 0);
		assert.ok(configFingerprint(text, []).includes('# keep this'));
	});
	it('blocks paths outside the target and detects case-insensitive Windows destination conflicts', () => {
		const files = [
			'../outside.json',
			'C:\\outside.json',
			'/outside.json',
			'nested/../outside.json',
			'',
			'bad?.json'
		].map((file_path) => ({ file_path, target: 'app_dir' as const, content: '' }));
		assert.equal(configurationProblems('.env', files).length, files.length);
		assert.equal(
			configurationProblems('.env', [{ file_path: '.ENV', target: 'app_dir', content: '' }]).length,
			1
		);
		assert.equal(
			fileLocation({ file_path: 'Settings\\APP.json', target: 'app_dir' }),
			'app_dir/settings/app.json'
		);
		assert.deepEqual(
			configurationProblems('.env', [
				{ file_path: 'app.json', target: 'app_dir', content: '' },
				{ file_path: 'app.json', target: 'release_dir', content: '' }
			]),
			[]
		);
	});
	it('does not share file objects with the server baseline and ignores database IDs for dirty state', () => {
		const baseline = [
			{ id: 9, file_path: 'config.json', target: 'app_dir' as const, content: '{"flag":true}' }
		];
		const draft = copyFiles(baseline);
		assert.equal(configFingerprint('', baseline), configFingerprint('', draft));
		draft[0].content = '{}';
		assert.equal(baseline[0].content, '{"flag":true}');
		assert.notEqual(configFingerprint('', baseline), configFingerprint('', draft));
	});
});
