import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import DiffView from './DiffView.svelte';

describe('DiffView', () => {
	it('shows a coordinate origin change in words (#1563)', () => {
		render(DiffView, {
			changes: {
				birth_place_origin: {
					old_value: { source: 'manual' },
					new_value: { source: 'geocoded', provider: 'Nominatim', precision: 'full' }
				}
			}
		});
		expect(screen.getByText('Entered by hand')).toBeDefined();
		expect(screen.getByText('Geocoded by Nominatim, full match')).toBeDefined();
	});
});
