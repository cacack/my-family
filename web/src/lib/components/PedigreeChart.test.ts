import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, cleanup } from '@testing-library/svelte';
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

const mockFourGenerations: PedigreeNode = {
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
			birth_date: { year: 1920 },
			father: {
				id: '8',
				given_name: 'James',
				surname: 'Doe',
				gender: 'male',
				birth_date: { year: 1890 }
			},
			mother: {
				id: '9',
				given_name: 'Sarah',
				surname: 'Williams',
				gender: 'female',
				birth_date: { year: 1895 }
			}
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

	afterEach(() => {
		// Clean up component and DOM
		cleanup();
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

	// Collapse/Expand functionality tests
	describe('collapse/expand functionality', () => {
		it('renders collapse toggle buttons on nodes with ancestors', () => {
			const { container } = render(PedigreeChart, { props: { data: mockPersonWithParents } });
			const toggleButtons = container.querySelectorAll('.collapse-toggle');
			// Root node (John) has ancestors, so should have a toggle
			expect(toggleButtons.length).toBe(1);
		});

		it('does not render collapse toggle on nodes without ancestors', () => {
			const { container } = render(PedigreeChart, { props: { data: mockSinglePerson } });
			const toggleButtons = container.querySelectorAll('.collapse-toggle');
			expect(toggleButtons.length).toBe(0);
		});

		it('shows minus sign by default for expanded branches', () => {
			const { container } = render(PedigreeChart, { props: { data: mockPersonWithParents } });
			const toggleText = container.querySelector('.toggle-text');
			expect(toggleText?.textContent).toBe('-');
		});

		it('renders multiple toggle buttons for multi-generation trees', () => {
			const { container } = render(PedigreeChart, { props: { data: mockThreeGenerations } });
			const toggleButtons = container.querySelectorAll('.collapse-toggle');
			// Root has ancestors, father has ancestors (2 grandparents)
			// Root (1 toggle) + Father (1 toggle) = 2 toggles
			expect(toggleButtons.length).toBe(2);
		});

		it('collapses branch when toggle is clicked', async () => {
			const { container, component } = render(PedigreeChart, { props: { data: mockThreeGenerations } });

			// Initial state: 5 nodes (John, Robert, Mary, William, Elizabeth)
			let nodes = container.querySelectorAll('.node');
			expect(nodes.length).toBe(5);

			// Call toggleCollapse on the father (Robert) to collapse his ancestors
			component.toggleCollapse('2');

			// Wait for debounce (50ms) + animation (300ms) + buffer
			await new Promise(resolve => setTimeout(resolve, 100));

			// After collapsing Robert's ancestors, check that collapsed state works
			expect(component.isCollapsed('2')).toBe(true);

			// The nodeMap should reflect the collapsed state
			expect(component.hasNode('4')).toBe(false); // William should not be in visible nodes
			expect(component.hasNode('5')).toBe(false); // Elizabeth should not be in visible nodes
			expect(component.hasNode('2')).toBe(true); // Robert should still be visible
		});

		it('expands branch when collapsed toggle is clicked', async () => {
			const { component } = render(PedigreeChart, { props: { data: mockThreeGenerations } });

			// Collapse first
			component.toggleCollapse('2');
			await new Promise(resolve => setTimeout(resolve, 100));

			expect(component.isCollapsed('2')).toBe(true);
			expect(component.hasNode('4')).toBe(false);

			// Expand again
			component.toggleCollapse('2');
			await new Promise(resolve => setTimeout(resolve, 100));

			// Should be expanded - grandparents visible again
			expect(component.isCollapsed('2')).toBe(false);
			expect(component.hasNode('4')).toBe(true); // William back in tree
			expect(component.hasNode('5')).toBe(true); // Elizabeth back in tree
		});

		it('shows ancestor count badge when collapsed', async () => {
			const { container, component } = render(PedigreeChart, { props: { data: mockThreeGenerations } });

			// Collapse the father's ancestors
			component.toggleCollapse('2');
			await new Promise(resolve => setTimeout(resolve, 100));

			// Should show ancestor badge
			const badge = container.querySelector('.ancestor-badge');
			expect(badge).not.toBeNull();

			// Badge should show +2 (William and Elizabeth)
			const badgeText = badge?.querySelector('text');
			expect(badgeText?.textContent).toBe('+2');
		});

		it('shows correct ancestor count for deeper trees', async () => {
			const { container, component } = render(PedigreeChart, { props: { data: mockFourGenerations } });

			// Collapse William's ancestors (has 2 great-grandparents)
			component.toggleCollapse('4');
			await new Promise(resolve => setTimeout(resolve, 100));

			const badges = container.querySelectorAll('.ancestor-badge');
			// William's badge should show +2 (James and Sarah)
			const badgeTexts = Array.from(badges).map(b => b.querySelector('text')?.textContent);
			expect(badgeTexts).toContain('+2');
		});

		it('shows plus sign when collapsed', async () => {
			const { container, component } = render(PedigreeChart, { props: { data: mockThreeGenerations } });

			// Collapse
			component.toggleCollapse('2');
			await new Promise(resolve => setTimeout(resolve, 100));

			// Find the toggle for Robert (the collapsed node)
			const toggleTexts = container.querySelectorAll('.toggle-text');
			const toggleTextContents = Array.from(toggleTexts).map(t => t.textContent);
			expect(toggleTextContents).toContain('+');
		});

		it('removes links when branch is collapsed', async () => {
			const { component } = render(PedigreeChart, { props: { data: mockThreeGenerations } });

			// Initial state: check that grandparents are in the tree
			expect(component.hasNode('4')).toBe(true); // William
			expect(component.hasNode('5')).toBe(true); // Elizabeth

			// Collapse Robert's ancestors
			component.toggleCollapse('2');
			await new Promise(resolve => setTimeout(resolve, 100));

			// After collapse, grandparents should not be in the node map
			// (links between Robert and his parents won't exist in the tree structure)
			expect(component.isCollapsed('2')).toBe(true);
			expect(component.hasNode('4')).toBe(false);
			expect(component.hasNode('5')).toBe(false);
		});

		it('does not trigger person click when clicking toggle', async () => {
			const clickHandler = vi.fn();
			const { container } = render(PedigreeChart, {
				props: { data: mockPersonWithParents, onPersonClick: clickHandler }
			});

			const toggleButton = container.querySelector('.collapse-toggle');
			expect(toggleButton).not.toBeNull();

			// Click the toggle button
			toggleButton?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
			await new Promise(resolve => setTimeout(resolve, 100));

			// Person click should not have been called because event.stopPropagation
			expect(clickHandler).not.toHaveBeenCalled();
		});

		it('isCollapsed returns correct state', async () => {
			const { component } = render(PedigreeChart, { props: { data: mockThreeGenerations } });

			// Initially not collapsed
			expect(component.isCollapsed('2')).toBe(false);

			// Collapse
			component.toggleCollapse('2');
			await new Promise(resolve => setTimeout(resolve, 100));

			// Now collapsed
			expect(component.isCollapsed('2')).toBe(true);

			// Expand
			component.toggleCollapse('2');
			await new Promise(resolve => setTimeout(resolve, 100));

			// Not collapsed again
			expect(component.isCollapsed('2')).toBe(false);
		});

		it('expandAll expands all collapsed branches', async () => {
			const { component } = render(PedigreeChart, { props: { data: mockFourGenerations } });

			// Collapse multiple branches
			component.toggleCollapse('2'); // Collapse Robert's ancestors
			await new Promise(resolve => setTimeout(resolve, 100));

			component.toggleCollapse('1'); // Collapse John's ancestors
			await new Promise(resolve => setTimeout(resolve, 100));

			expect(component.isCollapsed('1')).toBe(true);
			expect(component.isCollapsed('2')).toBe(true);

			// Expand all
			component.expandAll();
			await new Promise(resolve => setTimeout(resolve, 100));

			// Should be back to full tree - all nodes visible
			expect(component.isCollapsed('1')).toBe(false);
			expect(component.isCollapsed('2')).toBe(false);
			expect(component.hasNode('4')).toBe(true); // William back in tree
			expect(component.hasNode('8')).toBe(true); // James back in tree
		});

		it('collapseAll collapses all nodes with ancestors', async () => {
			const { component } = render(PedigreeChart, { props: { data: mockThreeGenerations } });

			// Collapse all
			component.collapseAll();
			await new Promise(resolve => setTimeout(resolve, 100));

			// All nodes with ancestors should be collapsed
			expect(component.isCollapsed('1')).toBe(true); // John collapsed
			expect(component.isCollapsed('2')).toBe(true); // Robert collapsed

			// Only root should be in the visible tree
			expect(component.hasNode('1')).toBe(true);
			expect(component.hasNode('2')).toBe(false); // Robert not visible (collapsed by John)
			expect(component.hasNode('3')).toBe(false); // Mary not visible
		});

		it('debounces rapid toggle operations', async () => {
			const { component } = render(PedigreeChart, { props: { data: mockThreeGenerations } });

			// Rapidly toggle multiple times (each call cancels the previous debounce)
			// The debounce is 50ms, so rapid calls should only trigger once
			component.toggleCollapse('2');
			component.toggleCollapse('2');
			component.toggleCollapse('2');

			// Immediately after, state should still be unchanged (debounce not fired)
			expect(component.isCollapsed('2')).toBe(false);

			// Wait for debounce to fire
			await new Promise(resolve => setTimeout(resolve, 100));

			// Final state should reflect last toggle (odd number of toggles = collapsed)
			expect(component.isCollapsed('2')).toBe(true);
			expect(component.hasNode('4')).toBe(false); // William not visible
		});
	});

	// Navigation tests with collapsed branches
	describe('navigation with collapsed branches', () => {
		it('getFatherId returns father ID from original data', async () => {
			const { component } = render(PedigreeChart, {
				props: { data: mockThreeGenerations, selectedPersonId: '1' }
			});

			// Father should be accessible initially
			expect(component.getFatherId()).toBe('2');

			// Collapse root (hide all ancestors)
			component.toggleCollapse('1');
			await new Promise(resolve => setTimeout(resolve, 100));

			// Father is not in visible nodes, but getFatherId works on original data
			// So it should still return the father ID from the data
			expect(component.getFatherId()).toBe('2');
		});

		it('hasNode returns false for collapsed ancestors', async () => {
			const { component } = render(PedigreeChart, { props: { data: mockThreeGenerations } });

			// Initially William should be in the tree
			expect(component.hasNode('4')).toBe(true);

			// Collapse Robert's ancestors
			component.toggleCollapse('2');
			await new Promise(resolve => setTimeout(resolve, 100));

			// Now William should not be in visible nodes
			expect(component.hasNode('4')).toBe(false);
		});
	});
});
