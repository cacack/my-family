import { afterAll, beforeAll, describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from '@testing-library/svelte';
import DescendancyChart from './DescendancyChart.svelte';
import type { DescendancyNode } from '#lib/api/client.js';
import { CARD_TEXT_INSET, MIN_CARD_FONT_PX } from '#lib/utils/chart.js';
import { mockTextLength } from '#lib/test/textLength.js';

// Card labels are cut to their measured width (#1213)
let restoreTextLength: () => void;
beforeAll(() => {
	restoreTextLength = mockTextLength();
});
afterAll(() => restoreTextLength());

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
			family_id: 'f2',
			name: 'Jane Smith',
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
			family_id: 'f10',
			name: 'Jane Smith'
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
			family_id: 'f2',
			name: 'Jane Smith',
			marriage_date: { year: 1975 }
		},
		{
			id: '3',
			family_id: 'f3',
			name: 'Sarah Johnson',
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

	it('places multiple spouses side by side in the person\'s row', () => {
		const { container } = render(DescendancyChart, { props: { data: mockMultipleSpouses } });
		const positions = Array.from(container.querySelectorAll('.node, .spouse-group')).map((el) => {
			const match = el.getAttribute('transform')?.match(/translate\(([^,]+),([^)]+)\)/);
			return { x: Number(match?.[1]), y: Number(match?.[2]) };
		});
		expect(positions).toHaveLength(3);
		expect(new Set(positions.map((p) => p.y)).size).toBe(1);
		expect(positions[1].x).toBeGreaterThan(positions[0].x);
		expect(positions[2].x).toBeGreaterThan(positions[1].x);
		expect(container.querySelectorAll('.spouse-link')).toHaveLength(2);
	});

	it('draws no text smaller than the fit policy assumes', () => {
		const { container } = render(DescendancyChart, { props: { data: mockMultipleSpouses } });
		for (const text of container.querySelectorAll('text')) {
			expect(parseFloat(text.getAttribute('font-size') ?? '')).toBeGreaterThanOrEqual(MIN_CARD_FONT_PX);
		}
	});

	it('renders BC years as "N BC", never as negative numbers (#1066)', () => {
		const bcPerson: DescendancyNode = {
			...mockPersonWithSpouse,
			birth_date: { year: -1850 },
			death_date: { year: -1790 },
			spouses: [{ id: '2', family_id: 'f2', name: 'Jane Smith', marriage_date: { year: -1825 } }]
		};
		const { container } = render(DescendancyChart, { props: { data: bcPerson } });
		const textContents = Array.from(container.querySelectorAll('.node text, .spouse-group text')).map(
			(t) => t.textContent
		);
		expect(textContents).toContain('1850 BC - 1790 BC');
		expect(textContents).toContain('m. 1825 BC');
		expect(textContents.join(' ')).not.toMatch(/-1[0-9]{3}/);
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

	it('draws each child link from the couple of the child\'s family (#1099)', () => {
		const data: DescendancyNode = {
			...mockMultipleSpouses,
			children: [
				{ id: '20', given_name: 'First', family_id: 'f2' },
				{ id: '30', given_name: 'Second', family_id: 'f3' }
			]
		};
		const { container } = render(DescendancyChart, { props: { data } });
		const links = Array.from(container.querySelectorAll('path.link'));
		const spouseOf = (childId: string) =>
			links.find((l) => l.getAttribute('data-child-id') === childId)?.getAttribute('data-spouse-id');
		expect(spouseOf('20')).toBe('2');
		expect(spouseOf('30')).toBe('3');

		// Each line starts between the person's row cards, at the second couple's connector for the second child
		const startX = (childId: string) =>
			parseFloat(
				links.find((l) => l.getAttribute('data-child-id') === childId)!.getAttribute('d')!.slice(1)
			);
		const spouseLinks = Array.from(container.querySelectorAll('.spouse-link')).map((l) => ({
			x1: parseFloat(l.getAttribute('x1')!),
			x2: parseFloat(l.getAttribute('x2')!)
		}));
		expect(startX('20')).toBeCloseTo((spouseLinks[0].x1 + spouseLinks[0].x2) / 2);
		expect(startX('30')).toBeCloseTo((spouseLinks[1].x1 + spouseLinks[1].x2) / 2);
	});

	it('numbers each marriage at its connector when a person has several spouses (#1099)', () => {
		const { container } = render(DescendancyChart, { props: { data: mockMultipleSpouses } });
		const badges = Array.from(container.querySelectorAll('.marriage-badge'));
		expect(badges.map((b) => b.querySelector('text')?.textContent)).toEqual(['1', '2']);
		expect(badges.map((b) => b.querySelector('title')?.textContent)).toEqual([
			'Marriage 1: John Doe and Jane Smith',
			'Marriage 2: John Doe and Sarah Johnson'
		]);

		const single = render(DescendancyChart, { props: { data: mockPersonWithSpouse } });
		expect(single.container.querySelectorAll('.marriage-badge')).toHaveLength(0);
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

	it('uses a neutral fill for spouse cards (no gender in the API)', () => {
		const { container } = render(DescendancyChart, { props: { data: mockPersonWithSpouse } });
		const spouseCard = container.querySelector('.spouse-group rect.spouse-card');
		expect(spouseCard?.getAttribute('fill')).toBe('#f1f5f9');
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
		expect(textContents).toContain('Jane Smith');
		expect(textContents).toContain('m. 1975');
		expect(container.querySelector('.spouse-group title')?.textContent).toBe('Jane Smith');
	});

	it('wraps long spouse names over two lines without a space before the ellipsis', () => {
		const data: DescendancyNode = {
			...mockSinglePerson,
			spouses: [{ id: '2', family_id: 'f2', name: 'Louise Margaret Alexandra Victoria Agnes of_Prussia' }]
		};
		const { container } = render(DescendancyChart, { props: { data } });
		const lines = Array.from(container.querySelectorAll('.spouse-group text')).map(
			(t) => t.textContent
		);
		expect(lines[0]).toBe('Louise Margaret');
		expect(lines[1]).toBe('Alexandra…');
		expect(lines[2]).toBe('');
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
		// Should be truncated with an ellipsis
		const truncatedTexts = textContents.filter((t) => t?.endsWith('…'));
		expect(truncatedTexts.length).toBeGreaterThan(0);
	});

	it('fits a 22-character unbroken given name and surname inside the card (#1213)', () => {
		const person: DescendancyNode = { id: '1', given_name: 'MWOLLSTONECRAFTWHEWELL', surname: 'MWOLLSTONECRAFTWHEWELL' };
		const { container } = render(DescendancyChart, { props: { data: person } });
		const width = Number(container.querySelector('.node-card')!.getAttribute('width'));
		const lines = Array.from(container.querySelectorAll('.node text')).slice(0, 2) as SVGTextElement[];
		for (const line of lines) {
			expect(line.textContent).toMatch(/…$/);
			expect(line.getComputedTextLength()).toBeLessThanOrEqual(width - 2 * CARD_TEXT_INSET);
		}
		expect(container.querySelector('.node title')?.textContent).toBe('MWOLLSTONECRAFTWHEWELL MWOLLSTONECRAFTWHEWELL');
	});

	it('fits a spouse with a 22-character unbroken surname inside the spouse card (#1213)', () => {
		const data: DescendancyNode = {
			...mockSinglePerson,
			spouses: [{ id: '2', family_id: 'f2', name: 'Ann MWOLLSTONECRAFTWHEWELL' }]
		};
		const { container } = render(DescendancyChart, { props: { data } });
		const width = Number(container.querySelector('.spouse-card')!.getAttribute('width'));
		const lines = Array.from(container.querySelectorAll('.spouse-group text')).slice(0, 2) as SVGTextElement[];
		expect(lines.map((t) => t.textContent)).toEqual(['Ann', expect.stringMatching(/^MWOLL.*…$/)]);
		for (const line of lines) {
			expect(line.getComputedTextLength()).toBeLessThanOrEqual(width - 2 * CARD_TEXT_INSET);
		}
		expect(container.querySelector('.spouse-group title')?.textContent).toBe('Ann MWOLLSTONECRAFTWHEWELL');
	});

	it('truncates at a word boundary and shows the full name as a tooltip', () => {
		const person: DescendancyNode = {
			id: '1',
			given_name: 'Beatrice Mary Victoria',
			surname: 'Saxe-Coburg'
		};
		const { container } = render(DescendancyChart, { props: { data: person } });
		const texts = Array.from(container.querySelectorAll('.node text')).map((t) => t.textContent);
		expect(texts).toContain('Beatrice Mary…');
		expect(container.querySelector('.node title')?.textContent).toBe(
			'Beatrice Mary Victoria Saxe-Coburg'
		);
	});

	it('leaves the surname line blank when there is no surname', () => {
		const person: DescendancyNode = { id: '1', given_name: 'Ælfgifu', surname: '' };
		const { container } = render(DescendancyChart, { props: { data: person } });
		const texts = Array.from(container.querySelectorAll('.node text')).map((t) => t.textContent);
		expect(texts).toContain('Ælfgifu');
		expect(texts).not.toContain('?');
		expect(container.querySelector('.node title')?.textContent).toBe('Ælfgifu');
	});

	it('leaves the given-name line blank when there is no given name', () => {
		const person: DescendancyNode = { id: '1', given_name: '', surname: 'Tudor' };
		const { container } = render(DescendancyChart, { props: { data: person } });
		const texts = Array.from(container.querySelectorAll('.node text')).map((t) => t.textContent);
		expect(texts.slice(0, 2)).toEqual(['', 'Tudor']);
		expect(texts).not.toContain('?');
		expect(container.querySelector('.node title')?.textContent).toBe('Tudor');
	});

	it('reads Unknown when there is no name at all', () => {
		const person: DescendancyNode = { id: '1', given_name: '', surname: '' };
		const { container } = render(DescendancyChart, { props: { data: person } });
		const texts = Array.from(container.querySelectorAll('.node text')).map((t) => t.textContent);
		expect(texts.slice(0, 2)).toEqual(['Unknown', '']);
		expect(texts).not.toContain('?');
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
