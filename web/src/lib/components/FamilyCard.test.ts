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

	it('falls back to "Unknown" when partner has empty given_name and surname', () => {
		// Issue #483: rendering now flows through formatPersonName, which trims
		// and filters both name parts and falls back to 'Unknown' when nothing
		// usable remains. This intentionally replaces the older ?? behavior that
		// would render a blank partner.
		const family: FamilyDetail = {
			id: 'fam-4',
			version: 1,
			partner1_id: 'p1',
			partner1: { id: 'p1', given_name: '', surname: '' }
		};

		render(FamilyCard, { props: { family } });

		expect(screen.getByText('Unknown')).toBeTruthy();
	});

	it('renders given_name and surname together as a single spaced string', () => {
		// Issue #483: backend now populates real surnames on FamilyDetail.partner*.
		// The card must combine both parts rather than reading just given_name.
		const family: FamilyDetail = {
			id: 'fam-5',
			version: 1,
			partner1_id: 'p1',
			partner2_id: 'p2',
			partner1: { id: 'p1', given_name: 'Jane', surname: 'Smith' },
			partner2: { id: 'p2', given_name: 'John', surname: 'Doe' }
		};

		render(FamilyCard, { props: { family } });

		expect(screen.getByText('Jane Smith')).toBeTruthy();
		expect(screen.getByText('John Doe')).toBeTruthy();
	});
});
