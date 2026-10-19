import { describe, it, expect, vi } from 'vitest';
import { SaveConflictFlow } from './saveConflict.svelte';

interface Saved {
	version: number;
	title: string;
	notes: string;
	tags: string[];
}
type Form = { title: string; notes: string; tags: string[] };

/** A minimal edit form: a record, the form's values, and a save spy. */
function setup(opened: Saved) {
	const state = { record: opened, form: formFrom(opened) };
	const save = vi.fn();
	function formFrom(saved: Saved): Form {
		return { title: saved.title, notes: saved.notes, tags: [...saved.tags] };
	}
	const flow = new SaveConflictFlow<Saved, Form>({
		base: () => state.record,
		formFrom,
		form: () => state.form,
		adopt: (latest, form) => {
			state.record = latest;
			state.form = form;
		},
		save
	});
	return { state, save, flow };
}

const opened: Saved = { version: 1, title: 'Census', notes: '', tags: ['a'] };
const latest: Saved = { version: 2, title: 'Census 1851', notes: 'Theirs', tags: ['a', 'b'] };

describe('SaveConflictFlow', () => {
	it('holds the latest version once it is re-read', async () => {
		const { flow } = setup(opened);
		expect(await flow.load(() => Promise.resolve(latest))).toBe(true);
		expect(flow.latest).toEqual(latest);
		flow.clear();
		expect(flow.latest).toBeNull();
	});

	it('says so when the latest version cannot be re-read', async () => {
		const { flow } = setup(opened);
		expect(await flow.load(() => Promise.reject(new Error('offline')))).toBe(false);
		expect(flow.latest).toBeNull();
	});

	it('lists only the edited fields, formatted', async () => {
		const { state, flow } = setup(opened);
		state.form = { ...state.form, title: 'Census, Kent' };
		expect(flow.fields([{ key: 'title', label: 'Title' }])).toEqual([]);
		await flow.load(() => Promise.resolve(latest));
		expect(
			flow.fields([
				{ key: 'title', label: 'Title', format: (value) => String(value).toUpperCase() },
				{ key: 'notes', label: 'Notes' },
				{ key: 'tags', label: 'Tags' }
			])
		).toEqual([{ label: 'Title', mine: 'CENSUS, KENT', latest: 'CENSUS 1851' }]);
	});

	it('says whether any field was edited', () => {
		const { state, flow } = setup(opened);
		expect(flow.hasEdits()).toBe(false);
		state.form = { ...state.form, tags: ['a', 'b'] };
		expect(flow.hasEdits()).toBe(true);
		state.form = { title: 'Census', notes: '', tags: ['a'] };
		expect(flow.hasEdits()).toBe(false);
	});

	it('joins a list for display', async () => {
		const { state, flow } = setup(opened);
		state.form = { ...state.form, tags: ['c'] };
		await flow.load(() => Promise.resolve(latest));
		expect(flow.fields([{ key: 'tags', label: 'Tags' }])).toEqual([{ label: 'Tags', mine: 'c', latest: 'a, b' }]);
	});

	it('saves the edits on the latest version, keeping its other changes', async () => {
		const { state, save, flow } = setup(opened);
		state.form = { ...state.form, title: 'Census, Kent' };
		await flow.load(() => Promise.resolve(latest));
		flow.keepMine();
		expect(state.record).toEqual(latest);
		expect(state.form).toEqual({ title: 'Census, Kent', notes: 'Theirs', tags: ['a', 'b'] });
		expect(save).toHaveBeenCalledOnce();
		expect(flow.latest).toBeNull();
	});

	it('drops the edits for the latest version', async () => {
		const { state, save, flow } = setup(opened);
		state.form = { ...state.form, title: 'Census, Kent' };
		await flow.load(() => Promise.resolve(latest));
		flow.useLatest();
		expect(state.record).toEqual(latest);
		expect(state.form).toEqual({ title: 'Census 1851', notes: 'Theirs', tags: ['a', 'b'] });
		expect(flow.latest).toBeNull();
		expect(save).not.toHaveBeenCalled();
	});

	it('does nothing without a conflict', () => {
		const { state, save, flow } = setup(opened);
		flow.keepMine();
		flow.useLatest();
		expect(state.record).toBe(opened);
		expect(save).not.toHaveBeenCalled();
	});
});
