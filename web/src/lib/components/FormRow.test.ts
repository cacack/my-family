import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import FormRow from './FormRow.svelte';

const fields = createRawSnippet(() => ({
	render: () => '<span><label>Given <input /></label><label>Surname <input /></label></span>'
}));

describe('FormRow', () => {
	it('renders its fields inside one row', () => {
		const { container } = render(FormRow, { props: { children: fields } });
		const row = container.querySelector('.form-row');
		expect(row).not.toBeNull();
		expect(row?.querySelectorAll('label')).toHaveLength(2);
	});

	it('defaults the minimum column width to 14rem', () => {
		const { container } = render(FormRow, { props: { children: fields } });
		const row = container.querySelector<HTMLElement>('.form-row');
		expect(row?.style.getPropertyValue('--form-row-min')).toBe('14rem');
	});

	it('takes a narrower minimum column width for rows of short fields', () => {
		const { container } = render(FormRow, {
			props: { children: fields, minColumnWidth: '10rem' }
		});
		const row = container.querySelector<HTMLElement>('.form-row');
		expect(row?.style.getPropertyValue('--form-row-min')).toBe('10rem');
	});
});
