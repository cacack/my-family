import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import PedigreeChart from './PedigreeChart.svelte';
import type { PedigreeNode } from '$lib/api/client';

// Mock data for tests
const mockSinglePerson: PedigreeNode = {
	id: '1',
	given_name: 'John',
	surname: 'Doe',
	gender: 'male',
	birth_date: { year: 1950 },
	death_date: { year: 2020 }
};

const mockPersonWithParents: PedigreeNode = {
	id: '1',
	given_name: 'John',
	surname: 'Doe',
	gender: 'male',
	birth_date: { year: 1950 },
	father: {
		id: '2',
		given_name: 'Robert',
		surname: 'Doe',
		gender: 'male',
		birth_date: { year: 1920 }
	},
	mother: {
		id: '3',
		given_name: 'Mary',
		surname: 'Smith',
		gender: 'female',
		birth_date: { year: 1925 }
	}
};

const mockThreeGenerations: PedigreeNode = {
	id: '1',
	given_name: 'John',
	surname: 'Doe',
	gender: 'male',
	birth_date: { year: 1980 },
	father: {
		id: '2',
		given_name: 'Robert',
		surname: 'Doe',
		gender: 'male',
		birth_date: { year: 1950 },
		father: {
			id: '4',
			given_name: 'William',
			surname: 'Doe',
			gender: 'male',
			birth_date: { year: 1920 }
		},
		mother: {
			id: '5',
			given_name: 'Elizabeth',
			surname: 'Johnson',
			gender: 'female',
			birth_date: { year: 1925 }
		}
	},
	mother: {
		id: '3',
		given_name: 'Mary',
		surname: 'Smith',
		gender: 'female',
		birth_date: { year: 1955 }
	}
};

describe('PedigreeChart', () => {
	beforeEach(() => {
		// Reset DOM
		document.body.innerHTML = '';
	});

	it('renders the component container', () => {
		const { container } = render(PedigreeChart, { props: { data: mockSinglePerson } });
		const chartContainer = container.querySelector('.pedigree-chart');
		expect(chartContainer).not.toBeNull();
	});

	it('creates an SVG element', () => {
		const { container } = render(PedigreeChart, { props: { data: mockSinglePerson } });
		const svg = container.querySelector('svg');
		expect(svg).not.toBeNull();
	});

	it('renders person nodes for single person', () => {
		const { container } = render(PedigreeChart, { props: { data: mockSinglePerson } });
		const nodes = container.querySelectorAll('.node');
		expect(nodes.length).toBe(1);
	});

	it('renders nodes for person with parents', () => {
		const { container } = render(PedigreeChart, { props: { data: mockPersonWithParents } });
		const nodes = container.querySelectorAll('.node');
		expect(nodes.length).toBe(3); // Person + 2 parents
	});

	it('renders correct number of nodes for three generations', () => {
		const { container } = render(PedigreeChart, { props: { data: mockThreeGenerations } });
		const nodes = container.querySelectorAll('.node');
		expect(nodes.length).toBe(5); // 1 + 2 + 2 (grandparents on father's side only)
	});

	it('renders link paths for relationships', () => {
		const { container } = render(PedigreeChart, { props: { data: mockPersonWithParents } });
		const links = container.querySelectorAll('.link');
		expect(links.length).toBe(2); // 2 links to parents
	});

	it('uses correct fill color for male nodes', () => {
		const { container } = render(PedigreeChart, { props: { data: mockSinglePerson } });
		const rect = container.querySelector('.node rect');
		expect(rect?.getAttribute('fill')).toBe('#dbeafe'); // Blue for male
	});

	it('uses correct fill color for female nodes', () => {
		const mockFemale: PedigreeNode = {
			id: '1',
			given_name: 'Jane',
			surname: 'Doe',
			gender: 'female'
		};
		const { container } = render(PedigreeChart, { props: { data: mockFemale } });
		const rect = container.querySelector('.node rect');
		expect(rect?.getAttribute('fill')).toBe('#fce7f3'); // Pink for female
	});

	it('supports different layout modes', () => {
		const { container: compactContainer } = render(PedigreeChart, {
			props: { data: mockSinglePerson, layout: 'compact' }
		});
		const compactRect = compactContainer.querySelector('.node rect');
		expect(compactRect?.getAttribute('width')).toBe('120'); // Compact card width

		document.body.innerHTML = '';

		const { container: wideContainer } = render(PedigreeChart, {
			props: { data: mockSinglePerson, layout: 'wide' }
		});
		const wideRect = wideContainer.querySelector('.node rect');
		expect(wideRect?.getAttribute('width')).toBe('160'); // Wide card width
	});

	it('renders name text in nodes', () => {
		const { container } = render(PedigreeChart, { props: { data: mockSinglePerson } });
		const texts = container.querySelectorAll('.node text');
		const textContents = Array.from(texts).map((t) => t.textContent);
		expect(textContents).toContain('John');
		expect(textContents).toContain('Doe');
	});

	it('renders birth-death dates', () => {
		const { container } = render(PedigreeChart, { props: { data: mockSinglePerson } });
		const texts = container.querySelectorAll('.node text');
		const textContents = Array.from(texts).map((t) => t.textContent);
		expect(textContents).toContain('1950 - 2020');
	});

	it('calls onPersonClick when node is clicked', async () => {
		const clickHandler = vi.fn();
		const { container } = render(PedigreeChart, {
			props: { data: mockSinglePerson, onPersonClick: clickHandler }
		});

		const node = container.querySelector('.node');
		expect(node).not.toBeNull();

		// Simulate click event
		node?.dispatchEvent(new MouseEvent('click', { bubbles: true }));

		expect(clickHandler).toHaveBeenCalledWith('1');
	});

	it('truncates long names with ellipsis', () => {
		const longNamePerson: PedigreeNode = {
			id: '1',
			given_name: 'Alexander-Christopher-William',
			surname: 'Van-Der-Berg-Smith-Johnson'
		};
		const { container } = render(PedigreeChart, { props: { data: longNamePerson } });
		const texts = container.querySelectorAll('.node text');
		const textContents = Array.from(texts).map((t) => t.textContent);
		// Should be truncated with ...
		const truncatedTexts = textContents.filter((t) => t?.includes('...'));
		expect(truncatedTexts.length).toBeGreaterThan(0);
	});

	it('handles missing dates gracefully', () => {
		const noDatesPerson: PedigreeNode = {
			id: '1',
			given_name: 'Unknown',
			surname: 'Person'
		};
		const { container } = render(PedigreeChart, { props: { data: noDatesPerson } });
		const texts = container.querySelectorAll('.node text');
		// Date line should be empty or not present
		const textContents = Array.from(texts).map((t) => t.textContent?.trim());
		const nonEmptyTexts = textContents.filter((t) => t && t.length > 0);
		expect(nonEmptyTexts).toContain('Unknown');
		expect(nonEmptyTexts).toContain('Person');
	});

	it('renders with birth only date format', () => {
		const birthOnlyPerson: PedigreeNode = {
			id: '1',
			given_name: 'Test',
			surname: 'Person',
			birth_date: { year: 1990 }
		};
		const { container } = render(PedigreeChart, { props: { data: birthOnlyPerson } });
		const texts = container.querySelectorAll('.node text');
		const textContents = Array.from(texts).map((t) => t.textContent);
		expect(textContents).toContain('b. 1990');
	});
});
