import { describe, it, expect, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import ConfirmDialog from './ConfirmDialog.svelte';

const svelteSources = import.meta.glob('/src/**/*.svelte', {
	query: '?raw',
	import: 'default',
	eager: true
}) as Record<string, string>;

function open(props: Partial<{ refusal: string | null; onconfirm: () => unknown }> = {}) {
	const onconfirm = props.onconfirm ?? vi.fn();
	render(ConfirmDialog, {
		open: true,
		title: 'Delete Ada Lovelace?',
		description: 'This cannot be undone.',
		...props,
		onconfirm
	});
	return { onconfirm, dialog: screen.getByRole('alertdialog') };
}

describe('ConfirmDialog', () => {
	it('asks the question and runs the step only once confirmed, then closes', async () => {
		const { onconfirm, dialog } = open();
		expect(dialog.textContent).toContain('Delete Ada Lovelace?');
		expect(dialog.textContent).toContain('This cannot be undone.');
		expect(onconfirm).not.toHaveBeenCalled();

		await fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));

		expect(onconfirm).toHaveBeenCalledOnce();
		await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
	});

	it('runs nothing when cancelled', async () => {
		const { onconfirm, dialog } = open();
		await fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));

		await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
		expect(onconfirm).not.toHaveBeenCalled();
	});

	it('stays open while the step runs, saying so', async () => {
		const { dialog } = open({ onconfirm: () => new Promise(() => {}) });
		await fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));

		const busy = await within(dialog).findByRole('button', { name: 'Deleting...' });
		expect(busy.hasAttribute('disabled')).toBe(true);
		expect(within(dialog).getByRole('button', { name: 'Cancel' }).hasAttribute('disabled')).toBe(true);
	});

	it('stays open and shows why when the step fails', async () => {
		const { dialog } = open({
			onconfirm: vi.fn().mockRejectedValue({ status: 409, message: 'Person is linked to families' })
		});
		await fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));

		expect((await within(dialog).findByRole('alert')).textContent).toContain('linked to families');
		expect(screen.getByRole('alertdialog')).toBeTruthy();
	});

	it('shows a known refusal instead of the question, with nothing to confirm', async () => {
		const { onconfirm, dialog } = open({ refusal: 'This family still has children.' });
		expect(dialog.textContent).toContain('This family still has children.');
		expect(dialog.textContent).not.toContain('This cannot be undone.');
		expect(within(dialog).queryByRole('button', { name: 'Delete' })).toBeNull();

		await fireEvent.click(within(dialog).getByRole('button', { name: 'Close' }));

		await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
		expect(onconfirm).not.toHaveBeenCalled();
	});

	// Every delete uses this dialog; the native one cannot show a refusal or a failure (#1113).
	it('is the only confirmation: no component calls the native confirm()', () => {
		const nativeConfirm = /(?:^|[^\w.]|window\.)confirm\s*\(/m;
		const callers = Object.entries(svelteSources)
			.filter(([, source]) => nativeConfirm.test(source))
			.map(([file]) => file);
		expect(callers).toEqual([]);
	});
});
