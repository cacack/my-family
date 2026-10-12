/**
 * Dates typed in plain English are saved in GEDCOM form and the edit form
 * says so; a date that can't be read is kept as text in one click (#978).
 * The create form does the same, and the new person's page says what a
 * converted date was saved as (#996).
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

test("the create form keeps an unreadable date as text and says what a converted one was saved as", async ({
  page,
  request,
}) => {
  await page.goto("/persons/add");

  await page.getByLabel("Given Name").fill("Datecreate");
  await page.getByLabel("Surname").fill(`Example${Date.now()}`);
  await page.getByLabel("Birth Date").fill("about 1850");
  await page.getByLabel("Death Date").fill("spring 1900");
  await page.getByRole("button", { name: "Create Person" }).click();
  await expect(page.getByRole("alert")).toContainText(
    "is not a recognized date",
  );
  await page.getByRole("button", { name: "Keep as text" }).click();

  await expect(page).toHaveURL(/\/persons\/[0-9a-f-]{36}$/);
  await expect(
    page.getByText("Saved as ABT 1850 from “about 1850”"),
  ).toBeVisible();
  await expect(page.getByText("(spring 1900)")).toBeVisible();

  const id = page.url().split("/").pop();
  const person = await (await request.get(`${API_BASE}/persons/${id}`)).json();
  expect(person.birth_date.raw).toBe("ABT 1850");
  expect(person.death_date.raw).toBe("(spring 1900)");
});
