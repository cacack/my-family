import { describe, it, expect, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import SaveConflict from './SaveConflict.svelte';

const fields = [
	{ label: 'Surname', mine: 'Lovelace', latest: 'Lovelace' },
	{ label: 'Birth Place', mine: 'Marylebone', latest: 'Westminster' },
	{ label: 'Notes', mine: '', latest: 'Added elsewhere' }
];

function renderConflict(props: Partial<{ busy: boolean }> = {}) {
	const onKeepMine = vi.fn();
	const onUseLatest = vi.fn();
	render(SaveConflict, { props: { noun: 'person', fields, onKeepMine, onUseLatest, ...props } });
	return { onKeepMine, onUseLatest };
}

describe('SaveConflict', () => {
	it('compares only the fields that differ', () => {
		renderConflict();
		expect(screen.queryByRole('rowheader', { name: 'Surname' })).toBeNull();
		const row = screen.getByRole('rowheader', { name: 'Birth Place' }).closest('tr')!;
		expect(row.textContent).toContain('Marylebone');
		expect(row.textContent).toContain('Westminster');
	});

	it('shows an empty value as a dash', () => {
		renderConflict();
		const row = screen.getByRole('rowheader', { name: 'Notes' }).closest('tr')!;
		expect(row.querySelector('[data-testid="mine"]')?.textContent).toBe('—');
	});

	it('says so when the edits already match', () => {
		render(SaveConflict, {
			props: { noun: 'family', fields: [fields[0]], onKeepMine: vi.fn(), onUseLatest: vi.fn() }
		});
		expect(screen.getByRole('alert').textContent).toContain('This family was changed elsewhere');
		expect(screen.getByText('Your edits match the latest saved version.')).toBeDefined();
		expect(screen.queryByRole('table')).toBeNull();
	});

	it('offers keeping the edits or taking the latest version', async () => {
		const { onKeepMine, onUseLatest } = renderConflict();
		await fireEvent.click(screen.getByRole('button', { name: 'Save my edits' }));
		await fireEvent.click(screen.getByRole('button', { name: 'Use the latest version' }));
		expect(onKeepMine).toHaveBeenCalledOnce();
		expect(onUseLatest).toHaveBeenCalledOnce();
	});

	it('disables both choices while saving', () => {
		renderConflict({ busy: true });
		expect((screen.getByRole('button', { name: 'Saving...' }) as HTMLButtonElement).disabled).toBe(true);
		expect(
			(screen.getByRole('button', { name: 'Use the latest version' }) as HTMLButtonElement).disabled
		).toBe(true);
	});
});
