/**
 * Dates typed in plain English are saved in GEDCOM form and the edit form
 * says so; a date that can't be read is kept as text in one click (#978).
 */
import { expect, test } from "@playwright/test";
import { API_BASE } from "./seed";

let personId = "";

test.beforeAll(async ({ request }) => {
  const response = await request.post(`${API_BASE}/persons`, {
    data: { given_name: "Dateinput", surname: `Example${Date.now()}` },
  });
  expect(response.status(), await response.text()).toBe(201);
  personId = (await response.json()).id;
});

test("an English date is saved as GEDCOM, and an unreadable one is kept as text", async ({
  page,
  request,
}) => {
  await page.goto(`/persons/${personId}`);

  await page.getByRole("button", { name: "Edit", exact: true }).first().click();
  await page.getByLabel("Birth Date").fill("about Sept 1850");
  await page.getByRole("button", { name: "Save Changes" }).click();
  await expect(
    page.getByText("Saved as ABT SEP 1850 from “about Sept 1850”"),
  ).toBeVisible();

  await page.getByRole("button", { name: "Edit", exact: true }).first().click();
  await page.getByLabel("Death Date").fill("spring 1900");
  await page.getByRole("button", { name: "Save Changes" }).click();
  await expect(page.getByRole("alert")).toContainText(
    "is not a recognized date",
  );
  await page.getByRole("button", { name: "Keep as text" }).click();
  await expect(page.getByText("(spring 1900)")).toBeVisible();

  const person = await (
    await request.get(`${API_BASE}/persons/${personId}`)
  ).json();
  expect(person.birth_date.raw).toBe("ABT SEP 1850");
  expect(person.death_date.raw).toBe("(spring 1900)");
});
