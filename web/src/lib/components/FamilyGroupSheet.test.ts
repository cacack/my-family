import { describe, it, expect } from 'vitest';
import { render, screen, within } from '@testing-library/svelte';
import FamilyGroupSheet from './FamilyGroupSheet.svelte';

describe('FamilyGroupSheet', () => {
	it('marks a living or private partner and child with named badges (#1413)', () => {
		render(FamilyGroupSheet, {
			props: {
				data: {
					id: 'f1',
					husband: { id: 'h', given_name: 'Hal', surname: 'Lee', living: false },
					wife: { id: 'w', given_name: 'Wren', surname: 'Lee', living: true, private: true },
					children: [{ id: 'c', given_name: 'Cal', surname: 'Lee', living: true }]
				}
			}
		});

		const row = (name: string) =>
			screen.getByRole('link', { name }).parentElement as HTMLElement;
		expect(within(row('Hal Lee')).queryByRole('img')).toBeNull();
		expect(within(row('Wren Lee')).getByRole('img', { name: 'Living person' })).toBeDefined();
		expect(within(row('Wren Lee')).getByRole('img', { name: 'Private person' })).toBeDefined();
		expect(within(row('Cal Lee')).getByRole('img', { name: 'Living person' })).toBeDefined();
		expect(within(row('Cal Lee')).queryByRole('img', { name: 'Private person' })).toBeNull();
	});
});
