import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import type { ExternalLink } from '$lib/api/client';
import ExternalLinks from './ExternalLinks.svelte';

describe('ExternalLinks', () => {
	it('renders a clickable link (new-tab, safe rel) when the identifier has a url', () => {
		const externalIds: ExternalLink[] = [
			{
				value: 'KWCJ-QN7',
				type: 'http://www.familysearch.org/ark',
				label: 'FamilySearch',
				url: 'https://www.familysearch.org/tree/person/details/KWCJ-QN7'
			}
		];
		render(ExternalLinks, { props: { externalIds } });

		const link = screen.getByRole('link', { name: /View on FamilySearch/ });
		expect(link.getAttribute('href')).toBe(
			'https://www.familysearch.org/tree/person/details/KWCJ-QN7'
		);
		expect(link.getAttribute('target')).toBe('_blank');
		expect(link.getAttribute('rel')).toBe('noopener noreferrer');
	});

	it('renders a plain label (no link) when the identifier has no url', () => {
		const externalIds: ExternalLink[] = [
			{ value: 'X99', type: 'http://example.com/unknown', label: 'http://example.com/unknown' }
		];
		render(ExternalLinks, { props: { externalIds } });

		expect(screen.queryByRole('link')).toBeNull();
		expect(screen.getByText(/X99/)).toBeDefined();
	});

	it('renders nothing when there are no external identifiers', () => {
		const { container } = render(ExternalLinks, { props: { externalIds: [] } });
		expect(container.querySelector('.external-links')).toBeNull();
	});
});
