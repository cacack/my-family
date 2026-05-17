import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import FamilyCard from './FamilyCard.svelte';
import type { FamilyDetail } from '$lib/api/client';

describe('FamilyCard', () => {
	it('renders both partner names from the nested partner objects', () => {
		const family: FamilyDetail = {
			id: 'fam-1',
			version: 1,
			partner1_id: 'p1',
			partner2_id: 'p2',
			partner1: { id: 'p1', given_name: 'John Smith', surname: '' },
			partner2: { id: 'p2', given_name: 'Jane Smith', surname: '' }
		};

		render(FamilyCard, { props: { family } });

		expect(screen.getByText('John Smith')).toBeTruthy();
		expect(screen.getByText('Jane Smith')).toBeTruthy();
	});

	it('renders single partner without an ampersand when partner2 is missing', () => {
		const family: FamilyDetail = {
			id: 'fam-2',
			version: 1,
			partner1_id: 'p1',
			partner1: { id: 'p1', given_name: 'Solo Parent', surname: '' }
		};

		render(FamilyCard, { props: { family } });

		expect(screen.getByText('Solo Parent')).toBeTruthy();
		expect(screen.queryByText('&')).toBeNull();
	});

	it('falls back to "Unknown" when partner1 is missing', () => {
		const family: FamilyDetail = {
			id: 'fam-3',
			version: 1
		};

		render(FamilyCard, { props: { family } });

		expect(screen.getByText('Unknown')).toBeTruthy();
	});

	it('renders an empty name verbatim rather than falling back to "Unknown"', () => {
		// ?? (nullish coalescing) only triggers on null/undefined, not on "".
		// A partner with a deliberately-empty name should render blank, not be
		// mislabeled "Unknown" — which would imply the partner record is absent.
		const family: FamilyDetail = {
			id: 'fam-4',
			version: 1,
			partner1_id: 'p1',
			partner1: { id: 'p1', given_name: '', surname: '' }
		};

		render(FamilyCard, { props: { family } });

		expect(screen.queryByText('Unknown')).toBeNull();
	});
});
