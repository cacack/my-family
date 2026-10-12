import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import SourcePage from './+page.svelte';
import * as apiModule from '$lib/api/client';

vi.mock('$lib/api/client', async (importOriginal) => {
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

vi.mock('$app/stores', () => ({
	page: {
		subscribe: vi.fn((callback: (value: unknown) => void) => {
			callback({ params: { id: 'src-1' } });
			return () => {};
		})
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
