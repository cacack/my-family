/**
 * Chart card labels in a real browser: names are cut to the card's measured
 * width, so a long unbroken name never spills out of its card, at Normal or
 * Larger font size (#1213). jsdom has no text layout, so only this suite can
 * check real glyph widths.
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

function useFontSize(page: Page, fontSize: string) {
	return page.addInitScript((size) => {
		localStorage.setItem('accessibility-settings', JSON.stringify({ fontSize: size }));
	}, fontSize);
}

// 22 characters with no break, wide digits included
const longName = `Chartwell${Date.now().toString().slice(-13)}`;
let rootId: string; // long given name and surname, married to Ann <long surname>
let childId: string; // their child, so the pedigree draws both parents

test.beforeAll(async ({ request }) => {
	expect(longName).toHaveLength(22);
	const root = await person(request, longName, longName, 'male');
	const spouse = await person(request, 'Ann', longName, 'female');
	const child = await person(request, longName, 'Short', 'female');
	const { id } = await post<{ id: string }>(request, '/families', {
		partner1_id: root,
		partner2_id: spouse,
		relationship_type: 'marriage'
	});
	await post(request, `/families/${id}/children`, { person_id: child });
	rootId = root;
	childId = child;
});

/** Each card's text lines that cross the card's edges, as "line text" strings. */
async function overflowingLines(page: Page, chart: string) {
	await expect(page.locator(`.${chart}-chart .node`).first()).toBeVisible();
	return page.evaluate((chart) => {
		const cards = [...document.querySelectorAll(`.${chart}-chart .node, .${chart}-chart .spouse-group`)];
		return cards.flatMap((card) => {
			const box = card.querySelector('rect.node-card, rect.spouse-card')!.getBoundingClientRect();
			return [...card.querySelectorAll(':scope > text')]
				.filter((text) => {
					const t = text.getBoundingClientRect();
					return t.width > 0 && (t.left < box.left - 0.5 || t.right > box.right + 0.5);
				})
				.map((text) => text.textContent);
		});
	}, chart);
}

for (const fontSize of ['normal', 'larger']) {
	for (const chart of ['descendancy', 'pedigree']) {
		test(`${chart} card names stay inside their cards at ${fontSize} font size (#1213)`, async ({ page }) => {
			await useFontSize(page, fontSize);
			await page.goto(`/${chart}/${chart === 'descendancy' ? rootId : childId}`);
			await expect(page.locator(`.${chart}-chart .node`)).toHaveCount(chart === 'descendancy' ? 2 : 3);
			expect(await overflowingLines(page, chart)).toEqual([]);
			// The full names stay available as the cards' tooltips
			const titles = page.locator(`.${chart}-chart title`);
			await expect(titles.filter({ hasText: `${longName} ${longName}` })).toHaveCount(1);
			if (chart === 'descendancy') {
				await expect(titles.filter({ hasText: `Ann ${longName}` })).toHaveCount(1);
			}
		});
	}
}
