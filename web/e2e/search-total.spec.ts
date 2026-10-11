/**
 * /search reports every match as the total, so a search with more matches than
 * one page offers Load more until all of them are shown (#902).
 */
import { expect, test } from "@playwright/test";
import { API_BASE } from "./seed";

const MATCHES = 25; // More than the 20-result first page
const surname = `Searchtotal${Date.now()}`;

test.beforeAll(async ({ request }) => {
  for (let i = 0; i < MATCHES; i++) {
    const response = await request.post(`${API_BASE}/persons`, {
      data: { given_name: `Match${String(i).padStart(2, "0")}`, surname },
    });
    expect(response.status(), await response.text()).toBe(201);
  }
});

test("a search with more than a page of matches loads more until it reaches every match", async ({
  page,
}) => {
  await page.goto(`/search?q=${surname}`);
  await expect(
    page.getByText(`Showing 20 of ${MATCHES} results`),
  ).toBeVisible();

  await page.getByRole("button", { name: "Load more" }).click();
  await expect(
    page.getByText(`Showing ${MATCHES} of ${MATCHES} results`),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Load more" })).toHaveCount(0);

  const links = page.locator(".results-table .person-link");
  await expect(links).toHaveCount(MATCHES);
  const hrefs = await links.evaluateAll((as) =>
    as.map((a) => a.getAttribute("href")),
  );
  expect(new Set(hrefs).size).toBe(MATCHES);
});
