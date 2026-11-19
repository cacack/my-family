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
		api.getPrivacySettings.mockResolvedValue({ living_threshold_years: 110, viewers_see_living: true });
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

	it('saves changed settings and announces it', async () => {
		api.updatePrivacySettings.mockResolvedValue({ living_threshold_years: 100, viewers_see_living: false });
		render(SettingsPage);
		await waitFor(() => expect(threshold().value).toBe('110'));

		await fireEvent.input(threshold(), { target: { value: '100' } });
		await fireEvent.click(viewers());
		await fireEvent.click(saveButton());

		expect(api.updatePrivacySettings).toHaveBeenCalledWith({ living_threshold_years: 100, viewers_see_living: false });
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

	it('tells a non-owner the page needs the owner, and loads nothing', async () => {
		signInAs('editor');
		render(SettingsPage);
		expect(screen.getByTestId('role-notice').textContent).toContain('needs the owner role');
		expect(api.getPrivacySettings).not.toHaveBeenCalled();
	});
});
