import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import SettingsPage from './+page.svelte';
import * as apiModule from '#lib/api/client.js';
import { signInAs } from '#lib/test/roles.js';
import { auth } from '#lib/stores/access.svelte.js';

const { goto } = vi.hoisted(() => ({ goto: vi.fn() }));
vi.mock('$app/navigation', () => ({ goto: (href: string) => goto(href) }));

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

	it('says what viewers see when the viewers setting is off', async () => {
		render(SettingsPage);
		await waitFor(() => expect(viewers()).toBeTruthy());
		expect(viewers().getAttribute('aria-describedby')).toBe('viewers-see-living-hint');
		const hint = document.getElementById('viewers-see-living-hint')?.textContent ?? '';
		expect(hint).toMatch(/see a living person as "Living"/);
		expect(hint).not.toMatch(/not enforced/i);
	});

	it('says what visitors see, and no longer that letting them in is unavailable', async () => {
		render(SettingsPage);
		await waitFor(() => expect(viewers()).toBeTruthy());
		const hint = document.getElementById('visitors-hint')?.textContent ?? '';
		expect(hint).toMatch(/Visitors will not see living people/);
		expect(hint).not.toMatch(/not available yet/i);
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

	it('links the visitors setting to the visitor preview (#1414)', async () => {
		render(SettingsPage);
		const group = await screen.findByRole('group', { name: 'Visitors who are not signed in' });
		const hint = document.getElementById(group.getAttribute('aria-describedby') ?? '');
		const link = hint?.querySelector('a');
		expect(link?.textContent).toBe('Preview the tree as a visitor under the saved settings');
		const target = document.getElementById(link?.getAttribute('href')?.slice(1) ?? '');
		expect(target?.querySelector('button')?.textContent?.trim()).toBe('Preview as a visitor');
	});

	it.each(['Preview as a visitor', 'Preview as a viewer'])(
		'%s waits until unsaved changes are saved, saying why (#1542)',
		async (name) => {
			api.updatePrivacySettings.mockResolvedValue({ living_threshold_years: 110, viewers_see_living: true, anonymous_access: 'viewer' });
			render(SettingsPage);
			const button = (await screen.findByRole('button', { name })) as HTMLButtonElement;
			expect(button.disabled).toBe(false);
			expect(document.getElementById('preview-unsaved')).toBeNull();

			await fireEvent.click(screen.getByLabelText('May browse the tree as viewers'));
			expect(button.disabled).toBe(true);
			const reason = document.getElementById('preview-unsaved');
			expect(reason?.textContent).toMatch(/unsaved changes/);
			expect(reason?.textContent).toMatch(/saved\s+settings/);
			expect(button.getAttribute('aria-describedby')?.split(' ')).toContain('preview-unsaved');
			await fireEvent.click(button);
			expect(auth.previewAs).toBeNull();
			expect(goto).not.toHaveBeenCalled();

			await fireEvent.click(saveButton());
			await waitFor(() => expect(button.disabled).toBe(false));
			expect(document.getElementById('preview-unsaved')).toBeNull();
			expect(button.getAttribute('aria-describedby')).toBe('preview-hint');
		}
	);

	it('tells a non-owner the page needs the owner, and loads nothing', async () => {
		signInAs('editor');
		render(SettingsPage);
		expect(screen.getByTestId('role-notice').textContent).toContain('needs the owner role');
		expect(api.getPrivacySettings).not.toHaveBeenCalled();
	});

	it.each([
		['Preview as a visitor', 'visitor'],
		['Preview as a viewer', 'viewer']
	] as const)('%s starts the preview and opens the home page (#1413)', async (name, as) => {
		render(SettingsPage);
		const button = await screen.findByRole('button', { name });
		await fireEvent.click(button);
		expect(auth.previewAs).toBe(as);
		expect(document.cookie).toContain(`myfamily_preview=${as}`);
		expect(goto).toHaveBeenCalledWith('/');
		document.cookie = 'myfamily_preview=; Path=/; Max-Age=0';
	});
});
