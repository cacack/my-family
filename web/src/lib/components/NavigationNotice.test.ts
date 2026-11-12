import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import NavigationNotice from './NavigationNotice.svelte';

const { pageState } = vi.hoisted(() => ({ pageState: {} as App.PageState }));

vi.mock('$app/state', () => ({
	get page() {
		return { state: pageState };
	}
}));

const listPages = import.meta.glob(
	['/src/routes/persons/+page.svelte', '/src/routes/families/+page.svelte', '/src/routes/sources/+page.svelte', '/src/routes/repositories/+page.svelte'],
	{ query: '?raw', import: 'default', eager: true }
) as Record<string, string>;

describe('NavigationNotice', () => {
	beforeEach(() => {
		delete pageState.notice;
	});

	it('shows the notice a delete left for the list it returned to', () => {
		pageState.notice = 'Ada Lovelace was deleted.';
		render(NavigationNotice);
		expect(screen.getByRole('status').textContent).toBe('Ada Lovelace was deleted.');
	});

	it('shows nothing without a notice', () => {
		render(NavigationNotice);
		expect(screen.queryByRole('status')).toBeNull();
	});

	// A delete returns to its list with this notice, the same for each (#1113).
	it('is shown by every list a delete returns to', () => {
		expect(Object.keys(listPages)).toHaveLength(4);
		for (const [file, source] of Object.entries(listPages)) {
			expect(source, file).toContain('<NavigationNotice />');
		}
	});
});
