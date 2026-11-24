import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import PlaceCoordinates from './PlaceCoordinates.svelte';

describe('PlaceCoordinates', () => {
	it('shows the pair and where it came from (#1563)', () => {
		render(PlaceCoordinates, {
			id: 'birth-place-coordinates',
			latitude: 'N42.3601',
			longitude: 'W71.0589',
			origin: { source: 'imported' }
		});
		const line = screen.getByTestId('place-coordinates');
		expect(line.id).toBe('birth-place-coordinates');
		expect(line.textContent?.replace(/\s+/g, ' ').trim()).toBe('N42.3601, W71.0589 · Imported');
	});

	it("names a geocoder's guess with its provider and precision", () => {
		render(PlaceCoordinates, {
			id: 'c',
			latitude: 'N42.36',
			longitude: 'W71.05',
			origin: { source: 'geocoded', provider: 'Nominatim', precision: 'partial' }
		});
		expect(screen.getByText('Geocoded by Nominatim, partial match')).toBeDefined();
	});

	it('renders nothing without a pair', () => {
		const { container } = render(PlaceCoordinates, { id: 'c', latitude: 'N1', longitude: null });
		expect(container.querySelector('.place-coordinates')).toBeNull();
	});
});
