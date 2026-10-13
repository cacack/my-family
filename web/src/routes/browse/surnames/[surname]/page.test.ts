import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/svelte";
import Page from "./+page.svelte";
import * as apiModule from "$lib/api/client";
import { currentPath, historyLength, resetRouter } from "$lib/test/fakeRouter";

vi.mock("$lib/api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof apiModule>();
  return { ...actual, api: { getPersonsBySurname: vi.fn() } };
});

vi.mock(
  "$app/stores",
  async () => (await import("$lib/test/fakeRouter")).appStores,
);
vi.mock(
  "$app/navigation",
  async () => (await import("$lib/test/fakeRouter")).appNavigation,
);

const person = {
  id: "p-1",
  given_name: "Ada",
  surname: "Lovelace",
  version: 1,
} as apiModule.Person;

describe("Browse surname detail page", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(apiModule.api.getPersonsBySurname).mockResolvedValue({
      items: [person],
      total: 45,
    });
  });

  it("moves a page past the end to the last page, replacing the URL (#948)", async () => {
    resetRouter("/browse/surnames/Lovelace?page=9", { surname: "Lovelace" });
    render(Page);
    await waitFor(() =>
      expect(currentPath()).toBe("/browse/surnames/Lovelace?page=3"),
    );
    expect(historyLength()).toBe(1);
    await screen.findByText("Page 3 of 3");
    expect(apiModule.api.getPersonsBySurname).toHaveBeenLastCalledWith(
      "Lovelace",
      { limit: 20, offset: 40 },
    );
  });
});
