import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import PersonCard from './PersonCard.svelte';

const ann = { id: 'ann-id', given_name: 'Ann', surname: 'Smith', gender: 'female' as const };
const john = { id: 'john-id', given_name: 'John', surname: 'Smith', gender: 'male' as const };

describe('PersonCard', () => {
	it('shows the new person when a reused card is given a different person', async () => {
		const { rerender } = render(PersonCard, { person: ann });
		expect(screen.getByText('Ann Smith')).toBeTruthy();

		await rerender({ person: john });

		expect(screen.getByText('John Smith')).toBeTruthy();
		expect(screen.queryByText('Ann Smith')).toBeNull();
	});
});
