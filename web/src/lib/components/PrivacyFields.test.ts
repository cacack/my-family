import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import PrivacyFields from './PrivacyFields.svelte';

function hintOf(label: string | RegExp): string {
	const control = screen.getByLabelText(label);
	const hint = document.getElementById(control.getAttribute('aria-describedby') ?? '');
	return hint?.textContent ?? '';
}

describe('PrivacyFields', () => {
	it.each(['family', 'source'])('says viewers and visitors do not see a private %s', (noun) => {
		render(PrivacyFields, { props: { noun, private: true } });
		const hint = hintOf(`Private ${noun}`);
		expect(hint).toMatch(new RegExp(`Viewers and visitors do not see a private ${noun}`));
		expect(hint).not.toMatch(/not enforced/);
	});

	it('says what visitors see of a living person', () => {
		render(PrivacyFields, {
			props: { noun: 'person', private: false, livingOverride: 'auto', living: true }
		});
		const hint = hintOf('Living status');
		expect(hint).toMatch(/counts this person as living/);
		expect(hint).toMatch(/Visitors see a living person as "Living"/);
		expect(hint).not.toMatch(/not enforced/);
	});
});
