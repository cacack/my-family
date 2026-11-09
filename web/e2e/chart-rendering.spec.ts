/**
 * Pedigree and descendancy rendering in a real browser: link layering after a
 * collapse and expand (#1025), keyboard-operable collapse toggles (#1100),
 * chart text following the Accessibility font-size setting (#1103) and the
 * first view pointing at first-generation relatives it cannot show (#1098).
 * The component tests cover the same in jsdom, which has no layout.
 */
import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { API_BASE } from './seed';

async function post<T>(request: APIRequestContext, path: string, data: unknown): Promise<T> {
	const response = await request.post(`${API_BASE}${path}`, { data });
	expect(response.status(), `POST ${path}: ${await response.text()}`).toBe(201);
	return (await response.json()) as T;
}

async function person(request: APIRequestContext, given_name: string, surname: string, gender = 'unknown') {
	return (await post<{ id: string }>(request, '/persons', { given_name, surname, gender })).id;
}

async function family(request: APIRequestContext, partner1_id: string, partner2_id: string, children: string[]) {
	const { id } = await post<{ id: string }>(request, '/families', {
		partner1_id,
		partner2_id,
		relationship_type: 'marriage'
	});
	for (const child of children) {
		await post(request, `/families/${id}/children`, { person_id: child });
	}
}

function useFontSize(page: Page, fontSize: string) {
	return page.addInitScript((size) => {
		localStorage.setItem('accessibility-settings', JSON.stringify({ fontSize: size }));
	}, fontSize);
}

let childId: string; // three generations of ancestors
let heirId: string; // four full generations: two parents, four grandparents, eight great-grandparents
let matriarchId: string; // nine children, each with a spouse and four children
// Spouse names stay short whatever the suffix: long unbroken names overflow a spouse card at any font size (#1213)
const surname = `Chart${Date.now().toString(36).slice(-4)}`;

test.beforeAll(async ({ request }) => {
	const p = (given: string, gender?: string) => person(request, given, surname, gender);
	const [gf, gm, father, mother, child] = [
		await p('Gideon', 'male'),
		await p('Greta', 'female'),
		await p('Felix', 'male'),
		await p('Mona', 'female'),
		await p('Cora', 'female')
	];
	await family(request, gf, gm, [father]);
	await family(request, father, mother, [child]);
	childId = child;

	// Each person's parents, recursively, so the tree is as wide as four generations get
	const ancestors = async (id: string, label: string, generations: number) => {
		if (generations === 0) return;
		const dad = await p(`${label}Fa`, 'male');
		const mum = await p(`${label}Mo`, 'female');
		await family(request, dad, mum, [id]);
		await ancestors(dad, `${label}Fa`, generations - 1);
		await ancestors(mum, `${label}Mo`, generations - 1);
	};
	heirId = await p('Heir');
	await ancestors(heirId, '', 3);

	matriarchId = await p('Matilda', 'female');
	const consort = await p('Consort', 'male');
	const children: string[] = [];
	for (let i = 0; i < 9; i++) {
		const kid = await p(`Kid${i}`);
		const spouse = await p(`Sp${i}`);
		const grandkids = [];
		for (let j = 0; j < 4; j++) grandkids.push(await p(`Grandkid${i}x${j}`));
		await family(request, kid, spouse, grandkids);
		children.push(kid);
	}
	await family(request, matriarchId, consort, children);
});

test('pedigree links stay below the cards after a collapse and expand (#1025)', async ({ page }) => {
	await page.goto(`/pedigree/${childId}`);
	const toggle = page.getByRole('button', { name: /ancestors of Felix/ });
	await toggle.click();
	await expect(page.locator('.node')).toHaveCount(3);
	await toggle.click();
	await expect(page.locator('.node')).toHaveCount(5);
	const linksFirst = await page.evaluate(() => {
		const nodes = [...document.querySelectorAll('.pedigree-chart .node')];
		return [...document.querySelectorAll('.pedigree-chart .link')].every((link) =>
			nodes.every((node) => link.compareDocumentPosition(node) & Node.DOCUMENT_POSITION_FOLLOWING)
		);
	});
	expect(linksFirst).toBe(true);
});

test('pedigree collapse toggles work from the keyboard (#1100)', async ({ page }) => {
	await page.goto(`/pedigree/${childId}`);
	await page.getByRole('application', { name: /Pedigree chart/ }).focus();
	await page.keyboard.press('Tab');

	const toggle = page.locator(':focus');
	await expect(toggle).toHaveAttribute('role', 'button');
	await expect(toggle).toHaveAttribute('aria-label', /^Collapse ancestors of Cora/);
	await expect(toggle).toHaveAttribute('aria-expanded', 'true');
	// The focus ring is drawn while the toggle has keyboard focus
	const ring = toggle.locator('.toggle-focus-ring');
	await expect.poll(() => ring.evaluate((c) => getComputedStyle(c).stroke)).not.toBe('none');

	await page.keyboard.press('Enter');
	await expect(toggle).toHaveAttribute('aria-expanded', 'false');
	await expect(toggle).toHaveAttribute('aria-label', /^Expand ancestors of Cora/);
	await expect(page.locator('.node')).toHaveCount(1);
	await expect(page).toHaveURL(new RegExp(`/pedigree/${childId}`));

	await page.keyboard.press('Space');
	await expect(toggle).toHaveAttribute('aria-expanded', 'true');
	await expect(page.locator('.node')).toHaveCount(5);
});

/** On-screen height of the first card's given name, whether any card text spills out of its card, and whether any two cards overlap. */
async function chartText(page: Page, chart: string) {
	await expect(page.locator(`.${chart}-chart .node`).first()).toBeVisible();
	// Let the fit settle
	await page.waitForTimeout(400);
	return page.evaluate((chart) => {
		const cards = [...document.querySelectorAll(`.${chart}-chart .node, .${chart}-chart .spouse-group`)];
		const overflow = cards.some((card) => {
			const box = card.querySelector('rect:not(.selection-ring)')!.getBoundingClientRect();
			return [...card.querySelectorAll(':scope > text')].some((text) => {
				const t = text.getBoundingClientRect();
				return t.width > 0 && (t.left < box.left - 0.5 || t.right > box.right + 0.5 || t.top < box.top - 0.5 || t.bottom > box.bottom + 0.5);
			});
		});
		const boxes = [...document.querySelectorAll(`.${chart}-chart rect.node-card, .${chart}-chart rect.spouse-card`)]
			.map((r) => r.getBoundingClientRect())
			.filter((b) => b.width > 0);
		const overlap = boxes.some((a, i) =>
			boxes.slice(i + 1).some((b) => a.left < b.right - 0.5 && b.left < a.right - 0.5 && a.top < b.bottom - 0.5 && b.top < a.bottom - 0.5)
		);
		const name = document.querySelector(`.${chart}-chart .node text`)!;
		const size = parseFloat(getComputedStyle(name).fontSize) * (name as SVGGraphicsElement).getScreenCTM()!.a;
		return { size, overflow, overlap };
	}, chart);
}

for (const chart of ['pedigree', 'descendancy']) {
	test(`${chart} text grows with the font-size setting, stays inside its cards and cards do not overlap (#1103)`, async ({ page }) => {
		const id = chart === 'pedigree' ? childId : matriarchId;
		const route = `/${chart}/${id}?gens=2`;
		await page.setViewportSize({ width: 1280, height: 900 });
		await page.goto(route);
		const normal = await chartText(page, chart);
		expect(normal.overlap, 'cards overlap at Normal').toBe(false);

		await useFontSize(page, 'larger');
		await page.goto(route);
		const larger = await chartText(page, chart);
		expect(larger.size).toBeGreaterThanOrEqual(normal.size * 1.25);
		expect(larger.overflow, 'card text overflows its card at Larger').toBe(false);
		expect(larger.overlap, 'cards overlap at Larger').toBe(false);
	});
}

test('quality chart text grows with the font-size setting (#1103)', async ({ page }) => {
	const size = async () => {
		const label = page.locator('.quality-chart .bar-label').first();
		await expect(label).toBeVisible();
		return label.evaluate((t) => parseFloat(getComputedStyle(t).fontSize));
	};
	await page.goto('/analytics');
	const normal = await size();
	await useFontSize(page, 'larger');
	await page.goto('/analytics');
	expect(await size()).toBeGreaterThanOrEqual(normal * 1.25);
});

for (const width of [375, 1280, 1440]) {
	test(`descendancy first view at ${width}px shows or points at all nine children (#1098)`, async ({ page }) => {
		await page.setViewportSize({ width, height: 900 });
		await page.goto(`/descendancy/${matriarchId}`);
		await expect(page.locator('.node').first()).toBeVisible();
		await page.waitForTimeout(400);

		const counted = async () => {
			const inView = await page.evaluate(() => {
				const view = document.querySelector('.descendancy-chart')!.getBoundingClientRect();
				return [...document.querySelectorAll<SVGGElement>('.descendancy-chart .node')]
					.filter((n) => (n as unknown as { __data__: { depth: number } }).__data__.depth === 1)
					.filter((n) => {
						const b = n.getBoundingClientRect();
						return b.right > view.left && b.left < view.right;
					}).length;
			});
			const side = async (s: string) => {
				const button = page.locator(`.offscreen-indicator.${s}`);
				return (await button.count()) ? Number((await button.textContent())!.match(/\d+/)![0]) : 0;
			};
			return { inView, left: await side('left'), right: await side('right') };
		};

		const first = await counted();
		expect(first.inView + first.left + first.right, JSON.stringify(first)).toBe(9);

		// Reset returns to the same first view after panning away
		const transform = () => page.locator('.descendancy-chart svg > g').getAttribute('transform');
		const initial = await transform();
		const side = first.right > 0 ? 'right' : 'left';
		await page.locator(`.offscreen-indicator.${side}`).click();
		await expect.poll(transform).not.toBe(initial);
		await page.getByRole('button', { name: 'Reset' }).click();
		await expect.poll(transform).toBe(initial);
		expect(await counted()).toEqual(first);
	});
}

/** The pedigree's parent cards: whether each lies inside the chart, and its given name as drawn. */
async function parentCards(page: Page) {
	await expect(page.locator('.pedigree-chart .node').first()).toBeVisible();
	// Let the fit settle
	await page.waitForTimeout(400);
	return page.evaluate(() => {
		const view = document.querySelector('.pedigree-chart')!.getBoundingClientRect();
		const inside = (b: DOMRect) =>
			b.left >= view.left && b.right <= view.right && b.top >= view.top && b.bottom <= view.bottom;
		return [...document.querySelectorAll<SVGGElement>('.pedigree-chart .node')]
			.filter((n) => (n as unknown as { __data__: { depth: number } }).__data__.depth === 1)
			.map((n) => {
				const name = n.querySelector<SVGTextElement>('.given-name')!;
				return {
					name: name.textContent,
					cardInView: inside(n.querySelector('.node-card')!.getBoundingClientRect()),
					nameInView: inside(name.getBoundingClientRect()),
					namePx: parseFloat(getComputedStyle(name).fontSize) * name.getScreenCTM()!.a
				};
			});
	});
}

test('pedigree first view at 375x667 shows both parents with readable names (#1098)', async ({ page }) => {
	await page.setViewportSize({ width: 375, height: 667 });
	await page.goto(`/pedigree/${heirId}`);
	const parents = await parentCards(page);
	expect(parents.map((p) => p.name).sort()).toEqual(['Fa', 'Mo']);
	for (const p of parents) {
		expect(p, JSON.stringify(p)).toMatchObject({ cardInView: true, nameInView: true });
		expect(p.namePx, JSON.stringify(p)).toBeGreaterThanOrEqual(10);
	}
	// With both parents in view, nothing points off-screen
	await expect(page.locator('.offscreen-indicator')).toHaveCount(0);

	// The generations that did not fit are folded behind the parents' +N badges
	const nodes = page.locator('.pedigree-chart .node');
	await expect(nodes).toHaveCount(3);
	await expect(page.locator('.ancestor-badge')).toHaveText(['+6', '+6']);

	// An expansion sticks until Reset, which returns to the same first view
	const transform = () => page.locator('.pedigree-chart svg > g').getAttribute('transform');
	const initial = await transform();
	await page.getByRole('button', { name: /^Expand ancestors of Fa / }).click();
	await expect(nodes).toHaveCount(9);
	await page.getByRole('button', { name: 'Reset' }).click();
	await expect(nodes).toHaveCount(3);
	await expect.poll(transform).toBe(initial);
});

for (const width of [1280, 1440]) {
	test(`pedigree first view at ${width}px draws all four generations (#1098)`, async ({ page }) => {
		await page.setViewportSize({ width, height: 900 });
		await page.goto(`/pedigree/${heirId}`);
		const parents = await parentCards(page);
		expect(parents.every((p) => p.cardInView), JSON.stringify(parents)).toBe(true);
		await expect(page.locator('.pedigree-chart .node')).toHaveCount(15);
		await expect(page.locator('.ancestor-badge')).toHaveCount(0);
	});
}
