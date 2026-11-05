import { describe, it as test } from 'node:test';
import assert from 'node:assert/strict';
import type { RollbackConfigService } from '$lib/api';
import { draftChanges, editableVersionService, isDirty, markSaved } from './version-config-model';

function service(overrides: Partial<RollbackConfigService>): RollbackConfigService {
	return {
		service_id: 1,
		service_name: 'api',
		env_file: '.env',
		snapshot_env: 'A=1',
		snapshot_files: [{ file_path: 'app.json', target: 'app_dir', content: '{}' }],
		has_draft: false,
		draft_env: '',
		draft_files: null,
		...overrides
	};
}

describe('version configuration drafts', () => {
	test('starts from the saved draft, keeping snapshot files when the draft has none', () => {
		const edited = editableVersionService(service({ has_draft: true, draft_env: 'A=2' }));
		assert.equal(edited.env, 'A=2');
		assert.deepEqual(edited.files, edited.snapshotFiles);
		assert.equal(isDirty(edited), false);
	});

	test('saves only services that differ from the snapshot, with explicit files', () => {
		const changed = editableVersionService(service({ service_id: 1 }));
		const untouched = editableVersionService(service({ service_id: 2 }));
		changed.env = 'A=3';
		const { save, discard } = draftChanges([changed, untouched]);
		assert.deepEqual(
			save.map((item) => [item.service_id, item.env_content, item.config_files.length]),
			[[1, 'A=3', 1]]
		);
		assert.deepEqual(discard, []);
	});

	test('a draft edited back to the snapshot is discarded', () => {
		const reverted = editableVersionService(service({ has_draft: true, draft_env: 'A=2' }));
		reverted.env = 'A=1';
		assert.deepEqual(draftChanges([reverted]), { save: [], discard: [1] });
		markSaved([reverted]);
		assert.equal(reverted.hasDraft, false);
		assert.equal(isDirty(reverted), false);
	});
});
