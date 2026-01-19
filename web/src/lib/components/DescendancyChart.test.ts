import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from '@testing-library/svelte';
import DescendancyChart from './DescendancyChart.svelte';
import type { DescendancyNode } from '$lib/api/client';

// Mock data for tests
const mockSinglePerson: DescendancyNode = {
	id: '1',
	given_name: 'John',
	surname: 'Doe',
	gender: 'male',
	birth_date: { year: 1950 },
	death_date: { year: 2020 }
};

const mockPersonWithSpouse: DescendancyNode = {
	id: '1',
	given_name: 'John',
	surname: 'Doe',
	gender: 'male',
	birth_date: { year: 1950 },
	spouses: [
		{
			id: '2',
			given_name: 'Jane',
			surname: 'Smith',
			gender: 'female',
			birth_date: { year: 1955 },
			marriage_date: { year: 1975 }
		}
	]
};

const mockPersonWithChildren: DescendancyNode = {
	id: '1',
	given_name: 'John',
	surname: 'Doe',
	gender: 'male',
	birth_date: { year: 1950 },
	children: [
		{
			id: '2',
			given_name: 'Robert',
			surname: 'Doe',
			gender: 'male',
			birth_date: { year: 1975 }
		},
		{
			id: '3',
			given_name: 'Mary',
			surname: 'Doe',
			gender: 'female',
			birth_date: { year: 1978 }
		}
	]
};

const mockThreeGenerations: DescendancyNode = {
	id: '1',
	given_name: 'John',
	surname: 'Doe',
	gender: 'male',
	birth_date: { year: 1920 },
	spouses: [
		{
			id: '10',
			given_name: 'Jane',
			surname: 'Smith',
			gender: 'female',
			birth_date: { year: 1925 }
		}
	],
	children: [
		{
			id: '2',
			given_name: 'Robert',
			surname: 'Doe',
			gender: 'male',
			birth_date: { year: 1950 },
			children: [
				{
					id: '4',
					given_name: 'William',
					surname: 'Doe',
					gender: 'male',
					birth_date: { year: 1980 }
				},
				{
					id: '5',
					given_name: 'Elizabeth',
					surname: 'Doe',
					gender: 'female',
					birth_date: { year: 1982 }
				}
			]
		},
		{
			id: '3',
			given_name: 'Mary',
			surname: 'Doe',
			gender: 'female',
			birth_date: { year: 1955 }
		}
	]
};

const mockMultipleSpouses: DescendancyNode = {
	id: '1',
	given_name: 'John',
	surname: 'Doe',
	gender: 'male',
	birth_date: { year: 1950 },
	spouses: [
		{
			id: '2',
			given_name: 'Jane',
			surname: 'Smith',
			gender: 'female',
			birth_date: { year: 1955 },
			marriage_date: { year: 1975 }
		},
		{
			id: '3',
			given_name: 'Sarah',
			surname: 'Johnson',
			gender: 'female',
			birth_date: { year: 1960 },
			marriage_date: { year: 1990 }
		}
	]
};

describe('DescendancyChart', () => {
	beforeEach(() => {
		// Reset DOM
		document.body.innerHTML = '';
	});

	it('renders the component container', () => {
		const { container } = render(DescendancyChart, { props: { data: mockSinglePerson } });
		const chartContainer = container.querySelector('.descendancy-chart');
		expect(chartContainer).not.toBeNull();
	});

	it('creates an SVG element', () => {
		const { container } = render(DescendancyChart, { props: { data: mockSinglePerson } });
		const svg = container.querySelector('svg');
		expect(svg).not.toBeNull();
	});

	it('renders person nodes for single person', () => {
		const { container } = render(DescendancyChart, { props: { data: mockSinglePerson } });
		const nodes = container.querySelectorAll('.node');
		expect(nodes.length).toBe(1);
	});

	it('renders nodes for person with children', () => {
		const { container } = render(DescendancyChart, { props: { data: mockPersonWithChildren } });
		const nodes = container.querySelectorAll('.node');
		expect(nodes.length).toBe(3); // Person + 2 children
	});

	it('renders correct number of nodes for three generations', () => {
		const { container } = render(DescendancyChart, { props: { data: mockThreeGenerations } });
		const nodes = container.querySelectorAll('.node');
		expect(nodes.length).toBe(5); // 1 + 2 + 2 (root + 2 children + 2 grandchildren)
	});

	it('renders spouse cards', () => {
		const { container } = render(DescendancyChart, { props: { data: mockPersonWithSpouse } });
		const spouseGroups = container.querySelectorAll('.spouse-group');
		expect(spouseGroups.length).toBe(1);
	});

	it('renders multiple spouse cards', () => {
		const { container } = render(DescendancyChart, { props: { data: mockMultipleSpouses } });
		const spouseGroups = container.querySelectorAll('.spouse-group');
		expect(spouseGroups.length).toBe(2);
	});

	it('renders spouse connector lines', () => {
		const { container } = render(DescendancyChart, { props: { data: mockPersonWithSpouse } });
		const spouseLinks = container.querySelectorAll('.spouse-link');
		expect(spouseLinks.length).toBe(1);
	});

	it('renders link paths for parent-child relationships', () => {
		const { container } = render(DescendancyChart, { props: { data: mockPersonWithChildren } });
		const links = container.querySelectorAll('.link');
		expect(links.length).toBe(2); // 2 links to children
	});

	it('uses correct fill color for male nodes', () => {
		const { container } = render(DescendancyChart, { props: { data: mockSinglePerson } });
		const rect = container.querySelector('.node rect.node-card');
		expect(rect?.getAttribute('fill')).toBe('#dbeafe'); // Blue for male
	});

	it('uses correct fill color for female nodes', () => {
		const mockFemale: DescendancyNode = {
			id: '1',
			given_name: 'Jane',
			surname: 'Doe',
			gender: 'female'
		};
		const { container } = render(DescendancyChart, { props: { data: mockFemale } });
		const rect = container.querySelector('.node rect.node-card');
		expect(rect?.getAttribute('fill')).toBe('#fce7f3'); // Pink for female
	});

	it('uses correct fill color for spouse cards', () => {
		const { container } = render(DescendancyChart, { props: { data: mockPersonWithSpouse } });
		const spouseCard = container.querySelector('.spouse-group rect.spouse-card');
		expect(spouseCard?.getAttribute('fill')).toBe('#fce7f3'); // Pink for female spouse
	});

	it('supports different layout modes', () => {
		const { container: compactContainer } = render(DescendancyChart, {
			props: { data: mockSinglePerson, layout: 'compact' }
		});
		const compactRect = compactContainer.querySelector('.node rect.node-card');
		expect(compactRect?.getAttribute('width')).toBe('120'); // Compact card width

		document.body.innerHTML = '';

		const { container: wideContainer } = render(DescendancyChart, {
			props: { data: mockSinglePerson, layout: 'wide' }
		});
		const wideRect = wideContainer.querySelector('.node rect.node-card');
		expect(wideRect?.getAttribute('width')).toBe('160'); // Wide card width
	});

	it('renders name text in nodes', () => {
		const { container } = render(DescendancyChart, { props: { data: mockSinglePerson } });
		const texts = container.querySelectorAll('.node text');
		const textContents = Array.from(texts).map((t) => t.textContent);
		expect(textContents).toContain('John');
		expect(textContents).toContain('Doe');
	});

	it('renders spouse name text', () => {
		const { container } = render(DescendancyChart, { props: { data: mockPersonWithSpouse } });
		const spouseTexts = container.querySelectorAll('.spouse-group text');
		const textContents = Array.from(spouseTexts).map((t) => t.textContent);
		expect(textContents).toContain('Jane');
		expect(textContents).toContain('Smith');
	});

	it('renders birth-death dates', () => {
		const { container } = render(DescendancyChart, { props: { data: mockSinglePerson } });
		const texts = container.querySelectorAll('.node text');
		const textContents = Array.from(texts).map((t) => t.textContent);
		expect(textContents).toContain('1950 - 2020');
	});

	it('calls onPersonClick when node is clicked', async () => {
		const clickHandler = vi.fn();
		const { container } = render(DescendancyChart, {
			props: { data: mockSinglePerson, onPersonClick: clickHandler }
		});

		const node = container.querySelector('.node');
		expect(node).not.toBeNull();

		// Simulate click event
		node?.dispatchEvent(new MouseEvent('click', { bubbles: true }));

		expect(clickHandler).toHaveBeenCalledWith('1');
	});

	it('calls onPersonClick when spouse card is clicked', async () => {
		const clickHandler = vi.fn();
		const { container } = render(DescendancyChart, {
			props: { data: mockPersonWithSpouse, onPersonClick: clickHandler }
		});

		const spouseGroup = container.querySelector('.spouse-group');
		expect(spouseGroup).not.toBeNull();

		// Simulate click event
		spouseGroup?.dispatchEvent(new MouseEvent('click', { bubbles: true }));

		expect(clickHandler).toHaveBeenCalledWith('2'); // Spouse ID
	});

	it('truncates long names with ellipsis', () => {
		const longNamePerson: DescendancyNode = {
			id: '1',
			given_name: 'Alexander-Christopher-William',
			surname: 'Van-Der-Berg-Smith-Johnson'
		};
		const { container } = render(DescendancyChart, { props: { data: longNamePerson } });
		const texts = container.querySelectorAll('.node text');
		const textContents = Array.from(texts).map((t) => t.textContent);
		// Should be truncated with ...
		const truncatedTexts = textContents.filter((t) => t?.includes('...'));
		expect(truncatedTexts.length).toBeGreaterThan(0);
	});

	it('handles missing dates gracefully', () => {
		const noDatesPerson: DescendancyNode = {
			id: '1',
			given_name: 'Unknown',
			surname: 'Person'
		};
		const { container } = render(DescendancyChart, { props: { data: noDatesPerson } });
		const texts = container.querySelectorAll('.node text');
		// Date line should be empty or not present
		const textContents = Array.from(texts).map((t) => t.textContent?.trim());
		const nonEmptyTexts = textContents.filter((t) => t && t.length > 0);
		expect(nonEmptyTexts).toContain('Unknown');
		expect(nonEmptyTexts).toContain('Person');
	});

	it('renders with birth only date format', () => {
		const birthOnlyPerson: DescendancyNode = {
			id: '1',
			given_name: 'Test',
			surname: 'Person',
			birth_date: { year: 1990 }
		};
		const { container } = render(DescendancyChart, { props: { data: birthOnlyPerson } });
		const texts = container.querySelectorAll('.node text');
		const textContents = Array.from(texts).map((t) => t.textContent);
		expect(textContents).toContain('b. 1990');
	});

	it('has accessible aria attributes', () => {
		const { container } = render(DescendancyChart, { props: { data: mockSinglePerson } });
		const chart = container.querySelector('.descendancy-chart');
		expect(chart?.getAttribute('role')).toBe('application');
		expect(chart?.getAttribute('aria-label')).toContain('Descendancy chart');
		expect(chart?.getAttribute('tabindex')).toBe('0');
	});

	it('renders children below parent in tree structure', () => {
		const { container } = render(DescendancyChart, { props: { data: mockPersonWithChildren } });
		const nodes = container.querySelectorAll('.node');

		// Get transforms to check Y positions
		const transforms = Array.from(nodes).map((node) => {
			const transform = node.getAttribute('transform');
			const match = transform?.match(/translate\(([^,]+),([^)]+)\)/);
			return match ? { x: parseFloat(match[1]), y: parseFloat(match[2]) } : null;
		}).filter(Boolean);

		// Root should be at or near y=0, children should have higher y values (below)
		const rootY = transforms[0]?.y || 0;
		const childrenY = transforms.slice(1).map((t) => t?.y || 0);

		// All children should be below the root (higher y value in SVG)
		childrenY.forEach((childY) => {
			expect(childY).toBeGreaterThan(rootY);
		});
	});

	it('exposes zoom methods', () => {
		const { component } = render(DescendancyChart, { props: { data: mockSinglePerson } });

		// Check that zoom methods are exposed
		expect(typeof component.zoomIn).toBe('function');
		expect(typeof component.zoomOut).toBe('function');
		expect(typeof component.resetZoom).toBe('function');
	});

	it('exposes navigation methods', () => {
		const { component } = render(DescendancyChart, { props: { data: mockThreeGenerations } });

		// Check that navigation methods are exposed
		expect(typeof component.getFirstChildId).toBe('function');
		expect(typeof component.getParentId).toBe('function');
		expect(typeof component.getRootId).toBe('function');
		expect(typeof component.getNextSiblingId).toBe('function');
		expect(typeof component.getPrevSiblingId).toBe('function');
		expect(typeof component.getSpouseId).toBe('function');
		expect(typeof component.hasNode).toBe('function');
	});

	it('returns correct root ID', () => {
		const { component } = render(DescendancyChart, { props: { data: mockThreeGenerations } });
		expect(component.getRootId()).toBe('1');
	});
});
