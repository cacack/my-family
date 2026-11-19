import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import PrivacyBadges from './PrivacyBadges.svelte';

describe('PrivacyBadges', () => {
	it('names a living person and a private record', () => {
		render(PrivacyBadges, { props: { living: true, private: true } });
		expect(screen.getByRole('img', { name: 'Living person' }).textContent).toContain('Living');
		expect(screen.getByRole('img', { name: 'Private person' }).textContent).toContain('Private');
	});

	it('names the private record by its noun', () => {
		render(PrivacyBadges, { props: { private: true, noun: 'family' } });
		expect(screen.getByRole('img', { name: 'Private family' })).toBeDefined();
		expect(screen.queryByRole('img', { name: 'Living person' })).toBeNull();
	});

	it('renders nothing for an unprotected record', () => {
		const { container } = render(PrivacyBadges, { props: { living: false } });
		expect(container.querySelector('.privacy-badges')).toBeNull();
	});
});
