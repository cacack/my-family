import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/svelte';
import { goto } from '$app/navigation';
import SourcePage from './+page.svelte';
import * as apiModule from '#lib/api/client.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			getSource: vi.fn(),
			updateSource: vi.fn(),
			deleteSource: vi.fn()
		}
	};
});

vi.mock('$app/state', () => ({
	get page() {
		return { params: { id: 'src-1' } };
	}
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

function source(overrides: Partial<apiModule.SourceDetail> = {}): apiModule.SourceDetail {
	return {
		id: 'src-1',
		source_type: 'census',
		title: '1851 Census',
		author: 'General Register Office',
		version: 1,
		...overrides
	} as apiModule.SourceDetail;
}

describe('Source detail page: save conflict (#939)', () => {
	const api = apiModule.api;
	const stale = { status: 409, code: 'VERSION_CONFLICT', message: 'Conflict' };

	beforeEach(() => {
		vi.clearAllMocks();
	});

	async function editTitle(value: string) {
		render(SourcePage);
		await fireEvent.click(await screen.findByRole('button', { name: 'Edit' }));
		await fireEvent.input(screen.getByLabelText(/^Title/), { target: { value } });
		await fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }));
	}

	it('compares the edits with the latest version and keeps them in the form', async () => {
		vi.mocked(api.getSource)
			.mockResolvedValueOnce(source())
			.mockResolvedValue(source({ title: '1851 Census of England', version: 2 }));
		vi.mocked(api.updateSource).mockRejectedValueOnce(stale);
		await editTitle('1851 Census, Kent');

		expect(await screen.findByRole('rowheader', { name: 'Title' })).toBeTruthy();
		expect(screen.getByTestId('mine').textContent).toBe('1851 Census, Kent');
		expect(screen.getByTestId('latest').textContent).toBe('1851 Census of England');
		expect((screen.getByLabelText(/^Title/) as HTMLInputElement).value).toBe('1851 Census, Kent');
		expect(screen.queryByText(/reload and try again/i)).toBeNull();
	});

	it("keeps another writer's changes to the fields the user did not edit", async () => {
		vi.mocked(api.getSource)
			.mockResolvedValueOnce(source())
			.mockResolvedValue(source({ author: 'Registrar General', notes: 'Theirs', version: 2 }));
		vi.mocked(api.updateSource).mockRejectedValueOnce(stale).mockResolvedValueOnce({} as apiModule.Source);
		await editTitle('1851 Census, Kent');

		expect(await screen.findByRole('rowheader', { name: 'Title' })).toBeTruthy();
		expect(screen.queryByRole('rowheader', { name: 'Author' })).toBeNull();

		await fireEvent.click(screen.getByRole('button', { name: 'Save my edits' }));

		await waitFor(() => expect(api.updateSource).toHaveBeenCalledTimes(2));
		expect(vi.mocked(api.updateSource).mock.calls[1][1]).toMatchObject({
			title: '1851 Census, Kent',
			author: 'Registrar General',
			notes: 'Theirs',
			version: 2
		});
	});

	it('replaces the edits with the latest version when the user takes it', async () => {
		vi.mocked(api.getSource)
			.mockResolvedValueOnce(source())
			.mockResolvedValue(source({ title: '1851 Census of England', version: 2 }));
		vi.mocked(api.updateSource).mockRejectedValueOnce(stale);
		await editTitle('1851 Census, Kent');

		await fireEvent.click(await screen.findByRole('button', { name: 'Use the latest version' }));

		expect((screen.getByLabelText(/^Title/) as HTMLInputElement).value).toBe('1851 Census of England');
		expect(screen.queryByText(/changed elsewhere/)).toBeNull();
		expect(api.updateSource).toHaveBeenCalledTimes(1);
	});

	it('shows a 409 that is not a change elsewhere as it came', async () => {
		vi.mocked(api.getSource).mockResolvedValue(source());
		vi.mocked(api.updateSource).mockRejectedValueOnce({
			status: 409,
			code: 'CONFLICT',
			message: 'Branch is merged and cannot be written to'
		});
		await editTitle('1851 Census, Kent');

		expect(await screen.findByText(/Branch is merged/)).toBeTruthy();
		expect(api.getSource).toHaveBeenCalledTimes(1);
		expect(screen.queryByText(/changed elsewhere/)).toBeNull();
	});
});

// GEDCOM CONT/CONC lines fold into these fields; a text input would strip the breaks (#1174).
describe('Source detail page: multi-line fields (#1174)', () => {
	const api = apiModule.api;

	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('saves an unchanged multi-line title, author and publisher with their line breaks', async () => {
		const multiline = {
			title: 'Parish register\nSt Mary, Kent',
			author: 'Church of England\nDiocese of Canterbury',
			publisher: 'Kent Archives\nMaidstone'
		};
		vi.mocked(api.getSource).mockResolvedValue(source(multiline));
		vi.mocked(api.updateSource).mockResolvedValue({} as apiModule.Source);
		render(SourcePage);

		await fireEvent.click(await screen.findByRole('button', { name: 'Edit' }));
		for (const label of [/^Title/, 'Author', 'Publisher']) {
			expect(screen.getByLabelText(label).tagName).toBe('TEXTAREA');
		}
		await fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }));

		await waitFor(() => expect(api.updateSource).toHaveBeenCalledTimes(1));
		expect(vi.mocked(api.updateSource).mock.calls[0][1]).toMatchObject(multiline);
	});
});

describe('Source detail page: save and delete feedback (#1113)', () => {
	const api = apiModule.api;

	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(api.getSource).mockResolvedValue(source());
	});

	it('announces a save in the live region', async () => {
		vi.mocked(api.updateSource).mockResolvedValue({} as apiModule.Source);
		render(SourcePage);
		await fireEvent.click(await screen.findByRole('button', { name: 'Edit' }));
		await fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }));

		await waitFor(() => expect(screen.getByTestId('announcer').textContent).toContain('Source saved'));
	});

	it('deletes only once the dialog is confirmed, then says so on the sources list', async () => {
		vi.mocked(api.deleteSource).mockResolvedValue(undefined);
		render(SourcePage);
		await fireEvent.click(await screen.findByRole('button', { name: 'Delete' }));

		const dialog = await screen.findByRole('alertdialog');
		expect(dialog.textContent).toContain('Delete "1851 Census"?');
		expect(api.deleteSource).not.toHaveBeenCalled();
		await fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));

		await waitFor(() => expect(api.deleteSource).toHaveBeenCalledWith('src-1', 1));
		expect(goto).toHaveBeenCalledWith('/sources', { state: { notice: '"1851 Census" was deleted.' } });
	});

	it('says a cited source cannot be deleted, instead of offering to', async () => {
		vi.mocked(api.getSource).mockResolvedValue(source({ citation_count: 2 }));
		render(SourcePage);
		await fireEvent.click(await screen.findByRole('button', { name: 'Delete' }));

		const dialog = await screen.findByRole('alertdialog');
		expect(dialog.textContent).toContain('This source is cited');
		expect(within(dialog).queryByRole('button', { name: 'Delete' })).toBeNull();
	});

	it('shows a refused delete in the dialog and stays on the source', async () => {
		vi.mocked(api.deleteSource).mockRejectedValue({ status: 409, message: 'Source has citations and cannot be deleted' });
		render(SourcePage);
		await fireEvent.click(await screen.findByRole('button', { name: 'Delete' }));
		const dialog = await screen.findByRole('alertdialog');
		await fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));

		expect((await within(dialog).findByRole('alert')).textContent).toContain('has citations');
		expect(goto).not.toHaveBeenCalled();
	});
});
