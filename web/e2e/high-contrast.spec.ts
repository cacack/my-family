/**
 * High-contrast mode keeps every piece of text readable (#905) and every text
 * link underlined in the link colour (#961).
 *
 * jsdom computes no colours, so only a real browser can tell whether the
 * high-contrast tokens actually reach the page. This turns the setting on the
 * way the Accessibility panel persists it and asks axe for colour-contrast
 * failures on the pages the original report covered. axe passes a link that
 * looks like body text, so the link checks read the computed style instead.
 */
import AxeBuilder from '@axe-core/playwright';
import { expect, test, type APIRequestContext, type Locator, type Page } from '@playwright/test';
import { API_BASE, readSeed } from './seed';

const PAGES: { name: string; path: (s: ReturnType<typeof readSeed>) => string }[] = [
	{ name: 'home', path: () => '/' },
	{ name: 'people', path: () => '/persons' },
	{ name: 'person', path: (s) => `/persons/${s.switcher.person.id}` },
	{ name: 'quality', path: () => '/quality' },
	{ name: 'evidence', path: () => '/evidence' },
	{ name: 'search', path: () => '/search' }
];

test.beforeEach(async ({ page }) => {
	await page.addInitScript(() => {
		localStorage.setItem(
			'accessibility-settings',
			JSON.stringify({ fontSize: 'normal', highContrast: true, reducedMotion: true })
		);
	});
});

async function expectNoContrastFailures(page: Page) {
	await expect(page.locator('body')).toHaveClass(/high-contrast/);
	const results = await new AxeBuilder({ page }).withRules(['color-contrast']).analyze();
	const failures = results.violations.flatMap((v) =>
		v.nodes.map((n) => `${n.target.join(' ')}: ${n.failureSummary ?? ''}`)
	);
	expect(failures).toEqual([]);
}

/** The high-contrast `--color-link`, as `getComputedStyle` reports it. */
const LINK_COLOUR = 'rgb(0, 0, 238)';

async function linkStyle(link: Locator) {
	return link.evaluate((el) => {
		const style = getComputedStyle(el);
		return { color: style.color, decoration: style.textDecorationLine };
	});
}

async function post<T>(request: APIRequestContext, path: string, data: unknown): Promise<T> {
	const response = await request.post(`${API_BASE}${path}`, { data });
	expect(response.status(), `POST ${path}: ${await response.text()}`).toBe(201);
	return (await response.json()) as T;
}

for (const { name, path } of PAGES) {
	test(`${name} has no colour-contrast failures in high-contrast mode`, async ({ page }) => {
		await page.goto(path(readSeed()));
		await page.waitForLoadState('networkidle');
		await expectNoContrastFailures(page);
	});
}

test('links styled as buttons keep their button colours', async ({ page }) => {
	await page.goto(`/persons/${readSeed().switcher.person.id}`);
	await page.waitForLoadState('networkidle');
	for (const name of ['Pedigree', 'Descendants', 'Ahnentafel']) {
		const style = await linkStyle(page.getByRole('link', { name, exact: true }));
		expect(style, name).toEqual({ color: 'rgb(0, 0, 0)', decoration: 'none' });
	}
});

/** WCAG relative luminance contrast ratio of two `rgb(r, g, b)` colours. */
function contrastRatio(a: string, b: string): number {
	const luminance = (rgb: string) => {
		const [r, g, bl] = (rgb.match(/\d+/g) ?? []).slice(0, 3).map((c) => {
			const v = Number(c) / 255;
			return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
		});
		return 0.2126 * r + 0.7152 * g + 0.0722 * bl;
	};
	const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
	return (hi + 0.05) / (lo + 0.05);
}

// The header icon's high-contrast rule once hid its active state (#918).
test('the header Advanced search icon shows it is the current page', async ({ page }) => {
	const icon = page.getByRole('link', { name: 'Advanced search' });
	const colours = () =>
		icon.evaluate((el) => {
			const style = getComputedStyle(el);
			return { background: style.backgroundColor, color: style.color };
		});

	await page.goto('/');
	await expect(page.locator('body')).toHaveClass(/high-contrast/);
	const idle = await colours();

	await page.goto('/search');
	await expect(icon).toHaveAttribute('aria-current', 'page');
	const active = await colours();
	expect(active.background).not.toEqual(idle.background);
	expect(contrastRatio(active.color, active.background)).toBeGreaterThanOrEqual(3);
});

/*
 * Both pages style their links with a component-scoped `a { ... }` rule, which
 * once beat the high-contrast link rule (#961). The data is this spec's own:
 * the shared seed has no child and no branch subject to link to.
 */
test.describe('component-styled text links', () => {
	let familyId: string;
	let branchId: string;
	let childName: string;
	let subjectName: string;

	test.beforeAll(async ({ request }) => {
		const surname = `Kontrast${Date.now().toString(36)}`;
		const parent = await post<{ id: string }>(request, '/persons', {
			given_name: 'Ida',
			surname
		});
		const child = await post<{ id: string }>(request, '/persons', {
			given_name: 'Otto',
			surname
		});
		const family = await post<{ id: string }>(request, '/families', { partner1_id: parent.id });
		await post(request, `/families/${family.id}/children`, { person_id: child.id });
		const branch = await post<{ id: string }>(request, '/branches', {
			name: `High contrast ${surname}`,
			hypothesis: `Was Ida ${surname} the ferry keeper?`,
			subjects: [{ type: 'person', id: parent.id }]
		});
		familyId = family.id;
		branchId = branch.id;
		childName = `Otto ${surname}`;
		subjectName = `Ida ${surname}`;
	});

	test('family detail: the children list links are underlined in the link colour', async ({
		page
	}) => {
		await page.goto(`/families/${familyId}`);
		const link = page.locator('.children-list').getByRole('link', { name: childName });
		await expect(link).toBeVisible();
		expect(await linkStyle(link)).toEqual({ color: LINK_COLOUR, decoration: 'underline' });
		await page.waitForLoadState('networkidle');
		await expectNoContrastFailures(page);
	});

	test('branch view: the research subject links are underlined in the link colour', async ({
		page
	}) => {
		await page.goto(`/branches/${branchId}`);
		const link = page.getByTestId('branch-research').getByRole('link', { name: subjectName });
		await expect(link).toBeVisible();
		expect(await linkStyle(link)).toEqual({ color: LINK_COLOUR, decoration: 'underline' });
		await page.waitForLoadState('networkidle');
		await expectNoContrastFailures(page);
	});
});
