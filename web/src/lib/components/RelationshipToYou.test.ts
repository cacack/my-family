import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import RelationshipToYou from './RelationshipToYou.svelte';
import * as apiModule from '#lib/api/client.js';
import { homePerson, loadHomePerson } from '#lib/stores/homePerson.svelte.js';
import { auth } from '#lib/stores/auth.svelte.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			getHomePerson: vi.fn(),
			setHomePerson: vi.fn(),
			clearHomePerson: vi.fn(),
			getRelationship: vi.fn()
		}
	};
});

const me = { id: 'me-1', given_name: 'Ada', surname: 'Home', version: 1 } as apiModule.Person;
const ancestor = { id: 'anc-1', gender: 'male' as const };

async function homeIs(person: apiModule.Person | null) {
	vi.mocked(apiModule.api.getHomePerson).mockResolvedValue(person ? { person } : {});
	await loadHomePerson();
}

function relationship(...names: string[]) {
	vi.mocked(apiModule.api.getRelationship).mockResolvedValue({
		paths: names.map((name) => ({ name }))
	} as apiModule.RelationshipResult);
}

describe('RelationshipToYou', () => {
	beforeEach(async () => {
		vi.clearAllMocks();
		await homeIs(me);
	});

	it('states the relationship to the home person, linked to the calculator', async () => {
		relationship('3rd great-grandparent');
		render(RelationshipToYou, { person: ancestor });

		const link = await screen.findByRole('link', { name: 'Your 3rd great-grandfather' });
		expect(link.getAttribute('href')).toBe('/relationship?personA=me-1&personB=anc-1');
		expect(apiModule.api.getRelationship).toHaveBeenCalledWith('me-1', 'anc-1');
	});

	it('says so when there is no known relationship', async () => {
		relationship();
		render(RelationshipToYou, { person: ancestor });
		expect(await screen.findByRole('link', { name: 'No known relationship to you' })).toBeTruthy();
	});

	it('says nothing about the relationship when it cannot be calculated', async () => {
		vi.mocked(apiModule.api.getRelationship).mockRejectedValue(new Error('boom'));
		render(RelationshipToYou, { person: ancestor });
		await waitFor(() => expect(apiModule.api.getRelationship).toHaveBeenCalled());
		expect(screen.queryByRole('link')).toBeNull();
		expect(screen.getByRole('button', { name: 'This is me' })).toBeTruthy();
	});

	it('marks the home person as you and clears them', async () => {
		vi.mocked(apiModule.api.clearHomePerson).mockResolvedValue();
		render(RelationshipToYou, { person: { id: 'me-1', gender: 'female' } });

		expect(screen.getByText('This is you')).toBeTruthy();
		expect(apiModule.api.getRelationship).not.toHaveBeenCalled();

		await fireEvent.click(screen.getByRole('button', { name: 'Not me' }));
		await waitFor(() => expect(homePerson.person).toBeNull());
		expect(await screen.findByRole('button', { name: 'This is me' })).toBeTruthy();
	});

	it('sets the person as you', async () => {
		await homeIs(null);
		const other = { ...me, id: 'anc-1' };
		vi.mocked(apiModule.api.setHomePerson).mockResolvedValue({ person: other });
		render(RelationshipToYou, { person: ancestor });

		await fireEvent.click(screen.getByRole('button', { name: 'This is me' }));
		expect(apiModule.api.setHomePerson).toHaveBeenCalledWith('anc-1');
		expect(await screen.findByText('This is you')).toBeTruthy();
	});

	it('does not offer a placeholder as you, but still states the relationship', async () => {
		relationship('3rd great-grandparent');
		render(RelationshipToYou, { person: { ...ancestor, redacted: true } });

		expect(await screen.findByRole('link', { name: 'Your 3rd great-grandfather' })).toBeTruthy();
		expect(screen.queryByRole('button', { name: 'This is me' })).toBeNull();
	});

	it('still lets you clear a home person who has become a placeholder', async () => {
		render(RelationshipToYou, { person: { id: 'me-1', gender: 'female', redacted: true } });

		expect(screen.getByText('This is you')).toBeTruthy();
		expect(screen.getByRole('button', { name: 'Not me' })).toBeTruthy();
	});

	it('shows a failed change inline', async () => {
		await homeIs(null);
		vi.mocked(apiModule.api.setHomePerson).mockRejectedValue(new Error('boom'));
		render(RelationshipToYou, { person: ancestor });

		await fireEvent.click(screen.getByRole('button', { name: 'This is me' }));
		expect((await screen.findByRole('alert')).textContent).toContain('Could not set who you are');
	});

	it('renders nothing until the home person is known, and reads a failed load as unset', async () => {
		vi.mocked(apiModule.api.getHomePerson).mockRejectedValue(new Error('offline'));
		await loadHomePerson();
		expect(homePerson.person).toBeNull();
		expect(homePerson.loaded).toBe(true);
	});
	it('lets any signed-in account change its own home person, whatever its role', async () => {
		auth.phase = 'signed-in';
		auth.account = { id: 'u-1', username: 'v', display_name: 'V', role: 'viewer' } as typeof auth.account;
		try {
			render(RelationshipToYou, { person: me });
			expect(screen.getByText('This is you')).toBeTruthy();
			expect(screen.getByRole('button', { name: 'Not me' })).toBeTruthy();
		} finally {
			auth.phase = 'open';
			auth.account = null;
		}
	});

	it('shows a visitor, who has no home person, neither control', async () => {
		await homeIs(null);
		relationship('3rd great-grandparent');
		auth.phase = 'visitor';
		try {
			render(RelationshipToYou, { person: ancestor });
			expect(screen.queryByRole('button', { name: 'This is me' })).toBeNull();
			expect(screen.queryByRole('button', { name: 'Not me' })).toBeNull();
			expect(apiModule.api.getRelationship).not.toHaveBeenCalled();
		} finally {
			auth.phase = 'open';
		}
	});
});
