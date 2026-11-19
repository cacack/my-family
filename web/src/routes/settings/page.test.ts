import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import SettingsPage from './+page.svelte';
import * as apiModule from '#lib/api/client.js';
import { signInAs } from '#lib/test/roles.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			getPrivacySettings: vi.fn(),
			updatePrivacySettings: vi.fn()
		}
	};
});

const api = vi.mocked(apiModule.api);

describe('Settings page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		signInAs('owner');
		api.getPrivacySettings.mockResolvedValue({ living_threshold_years: 110, viewers_see_living: true, anonymous_access: 'none' });
	});

	const threshold = () => screen.getByLabelText('Living threshold (years)') as HTMLInputElement;
	const viewers = () => screen.getByLabelText('Viewers see living people') as HTMLInputElement;
	const saveButton = () => screen.getByRole('button', { name: 'Save settings' }) as HTMLButtonElement;

	it('shows the saved settings, with nothing to save until one changes', async () => {
		render(SettingsPage);
		await waitFor(() => expect(threshold().value).toBe('110'));
		expect(viewers().checked).toBe(true);
		expect(threshold().min).toBe('50');
		expect(threshold().max).toBe('150');
		expect(saveButton().disabled).toBe(true);
	});

	it('says the viewers setting is not enforced yet', async () => {
		render(SettingsPage);
		await waitFor(() => expect(viewers()).toBeTruthy());
		expect(viewers().getAttribute('aria-describedby')).toBe('viewers-see-living-hint');
		expect(document.getElementById('viewers-see-living-hint')?.textContent).toMatch(/not enforced yet/i);
	});

	it('saves changed settings and announces it', async () => {
		api.updatePrivacySettings.mockResolvedValue({ living_threshold_years: 100, viewers_see_living: false, anonymous_access: 'none' });
		render(SettingsPage);
		await waitFor(() => expect(threshold().value).toBe('110'));

		await fireEvent.input(threshold(), { target: { value: '100' } });
		await fireEvent.click(viewers());
		await fireEvent.click(saveButton());

		expect(api.updatePrivacySettings).toHaveBeenCalledWith({
			living_threshold_years: 100,
			viewers_see_living: false,
			anonymous_access: 'none'
		});
		await waitFor(() => expect(screen.getByTestId('announcer').textContent).toContain('Privacy settings saved'));
		expect(saveButton().disabled).toBe(true);
	});

	it('shows a refused save and keeps what was entered', async () => {
		api.updatePrivacySettings.mockRejectedValue({ status: 400, message: 'living threshold must be between 50 and 150 years, not 40' });
		render(SettingsPage);
		await waitFor(() => expect(threshold().value).toBe('110'));

		await fireEvent.input(threshold(), { target: { value: '120' } });
		await fireEvent.click(saveButton());

		expect(await screen.findByText(/must be between 50 and 150/)).toBeDefined();
		expect(threshold().value).toBe('120');
	});

	it('offers visitor access, saying what visitors will and will not see', async () => {
		api.updatePrivacySettings.mockResolvedValue({ living_threshold_years: 110, viewers_see_living: true, anonymous_access: 'viewer' });
		render(SettingsPage);
		const closed = (await screen.findByLabelText('Must sign in to see anything')) as HTMLInputElement;
		const open = screen.getByLabelText('May browse the tree as viewers') as HTMLInputElement;
		expect(closed.checked).toBe(true);
		const group = screen.getByRole('group', { name: 'Visitors who are not signed in' });
		const hint = document.getElementById(group.getAttribute('aria-describedby') ?? '')?.textContent ?? '';
		expect(hint).toMatch(/will see people who have died/);
		expect(hint).toMatch(/will not see living people/);

		await fireEvent.click(open);
		await fireEvent.click(saveButton());
		expect(api.updatePrivacySettings).toHaveBeenCalledWith({
			living_threshold_years: 110,
			viewers_see_living: true,
			anonymous_access: 'viewer'
		});
	});

	it('tells a non-owner the page needs the owner, and loads nothing', async () => {
		signInAs('editor');
		render(SettingsPage);
		expect(screen.getByTestId('role-notice').textContent).toContain('needs the owner role');
		expect(api.getPrivacySettings).not.toHaveBeenCalled();
	});
});
