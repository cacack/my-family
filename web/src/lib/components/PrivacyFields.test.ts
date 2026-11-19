import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import PrivacyFields from './PrivacyFields.svelte';

function hintOf(label: string | RegExp): string {
	const control = screen.getByLabelText(label);
	const hint = document.getElementById(control.getAttribute('aria-describedby') ?? '');
	return hint?.textContent ?? '';
}

describe('PrivacyFields', () => {
	it('says viewers do not see a private record of a kind that is hidden from them', () => {
		render(PrivacyFields, { props: { noun: 'family', private: true, hiddenFromViewers: true } });
		const hint = hintOf('Private family');
		expect(hint).toMatch(/Viewers and visitors do not see a private family/);
		expect(hint).not.toMatch(/not enforced/);
	});

	it('says hiding is not enforced yet for other kinds of record', () => {
		render(PrivacyFields, { props: { noun: 'source', private: false } });
		expect(hintOf('Private source')).toMatch(/not enforced yet/);
	});

	it('says what visitors see of a living person', () => {
		render(PrivacyFields, {
			props: { noun: 'person', private: false, livingOverride: 'auto', living: true, hiddenFromViewers: true }
		});
		const hint = hintOf('Living status');
		expect(hint).toMatch(/counts this person as living/);
		expect(hint).toMatch(/Visitors see a living person as "Living"/);
		expect(hint).not.toMatch(/not enforced/);
	});
});
