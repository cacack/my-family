import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { createRawSnippet, flushSync } from 'svelte';
import InfoSection from './InfoSection.svelte';

const snippet = (html: string) => createRawSnippet(() => ({ render: () => html }));

function rows(container: HTMLElement) {
	return [...container.querySelectorAll('dt')].map((dt) => [dt.textContent, dt.nextElementSibling?.textContent?.trim()]);
}

describe('InfoSection', () => {
	it('labels the section with an h2', () => {
		render(InfoSection, { props: { title: 'Birth' } });
		expect(screen.getByRole('heading', { level: 2, name: 'Birth' })).toBeDefined();
	});

	it('renders fields as dt/dd pairs, showing — for an empty value', () => {
		const { container } = render(InfoSection, {
			props: {
				title: 'Birth',
				fields: [
					{ label: 'Date', value: '1 JAN 1850' },
					{ label: 'Place', value: '' },
					{ label: 'Source', value: null },
					{ label: 'Notes' }
				]
			}
		});
		expect(rows(container)).toEqual([
			['Date', '1 JAN 1850'],
			['Place', '—'],
			['Source', '—'],
			['Notes', '—']
		]);
	});

	it('leaves out empty fields with hideEmpty', () => {
		const { container } = render(InfoSection, {
			props: {
				title: 'Address',
				hideEmpty: true,
				fields: [
					{ label: 'City', value: 'Springfield' },
					{ label: 'Fax', value: undefined }
				]
			}
		});
		expect(rows(container)).toEqual([['City', 'Springfield']]);
	});

	it('renders no list when there are no fields', () => {
		const { container } = render(InfoSection, { props: { title: 'Notes', children: snippet('<p>Some notes</p>') } });
		expect(container.querySelector('dl')).toBeNull();
		expect(screen.getByText('Some notes')).toBeDefined();
	});

	it('shows a field note as a status line under the value', () => {
		render(InfoSection, {
			props: { title: 'Birth', fields: [{ label: 'Date', value: 'ABT 1850', note: 'Saved as ABT 1850' }] }
		});
		const note = screen.getByRole('status');
		expect(note.textContent).toBe('Saved as ABT 1850');
		expect(note.closest('dd')).not.toBeNull();
	});

	it('puts a badge inside the heading and actions beside it', () => {
		render(InfoSection, {
			props: {
				title: 'Media',
				badge: snippet('<span>3</span>'),
				actions: snippet('<button>Add</button>')
			}
		});
		const heading = screen.getByRole('heading', { level: 2 });
		expect(heading.textContent).toContain('Media');
		expect(heading.textContent).toContain('3');
		expect(heading.contains(screen.getByRole('button', { name: 'Add' }))).toBe(false);
	});

	it('focuses its heading on request', () => {
		const { component } = render(InfoSection, { props: { title: 'Children' } });
		flushSync(() => component.focusHeading());
		expect(document.activeElement).toBe(screen.getByRole('heading', { level: 2, name: 'Children' }));
	});
});
