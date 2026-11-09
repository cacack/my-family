import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, waitFor, fireEvent, screen } from "@testing-library/svelte";
import AhnentafelPage from "./+page.svelte";
import * as apiModule from "#lib/api/client.js";
import { resetRouter } from "#lib/test/fakeRouter.svelte.js";

vi.mock("#lib/api/client.js", async (importOriginal) => {
  const actual = await importOriginal<typeof apiModule>();
  return {
    ...actual,
    api: { getAhnentafel: vi.fn(), getAhnentafelText: vi.fn() },
  };
});

vi.mock(
  "$app/state",
  async () => (await import("#lib/test/fakeRouter.svelte.js")).appState,
);
vi.mock(
  "$app/navigation",
  async () => (await import("#lib/test/fakeRouter.svelte.js")).appNavigation,
);

function reportWith(
  entry: Partial<apiModule.AhnentafelEntry>,
): apiModule.AhnentafelResponse {
  return {
    subject: {
      id: "p-1",
      given_name: entry.given_name ?? "",
      surname: entry.surname ?? "",
    },
    entries: [
      { number: 1, generation: 0, id: "p-1", relationship: "Self", ...entry },
    ],
    generations: 1,
    total_count: 1,
    known_count: 1,
    ancestor_slots: 0,
    known_ancestors: 0,
  } as apiModule.AhnentafelResponse;
}

async function nameLinks(
  entry: Partial<apiModule.AhnentafelEntry>,
): Promise<string[]> {
  vi.mocked(apiModule.api.getAhnentafel).mockResolvedValue(reportWith(entry));
  resetRouter("/ahnentafel/p-1", { id: "p-1" });
  const { container } = render(AhnentafelPage);
  await waitFor(() =>
    expect(container.querySelector(".person-link")).not.toBeNull(),
  );
  return Array.from(container.querySelectorAll(".person-link")).map((a) =>
    (a.textContent ?? "").trim(),
  );
}

describe("Ahnentafel page: missing name parts (#932)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("leaves out a missing surname", async () => {
    const names = await nameLinks({ given_name: "Ælfgifu", surname: "" });
    expect(names.length).toBeGreaterThan(0);
    for (const name of names) expect(name).toBe("Ælfgifu");
  });

  it("leaves out a missing given name", async () => {
    const names = await nameLinks({ given_name: "", surname: "Tudor" });
    for (const name of names) expect(name).toBe("Tudor");
  });

  it("reads Unknown when there is no name at all", async () => {
    const names = await nameLinks({});
    for (const name of names) expect(name).toBe("Unknown");
  });
});

describe("Ahnentafel page: export filename follows the display name (#932)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  async function exportFilename(
    entry: Partial<apiModule.AhnentafelEntry>,
  ): Promise<string> {
    vi.mocked(apiModule.api.getAhnentafel).mockResolvedValue(reportWith(entry));
    vi.mocked(apiModule.api.getAhnentafelText).mockResolvedValue("report");
    URL.createObjectURL = vi.fn(() => "blob:x");
    URL.revokeObjectURL = vi.fn();
    let download = "";
    const click = vi
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(function (this: HTMLAnchorElement) {
        download = this.download;
      });
    resetRouter("/ahnentafel/p-1", { id: "p-1" });
    const { container } = render(AhnentafelPage);
    await waitFor(() =>
      expect(container.querySelector(".person-link")).not.toBeNull(),
    );
    await fireEvent.click(screen.getByRole("button", { name: "Export Text" }));
    await waitFor(() => expect(click).toHaveBeenCalled());
    click.mockRestore();
    return download;
  }

  it.each([
    [
      { given_name: "Mary Ann", surname: "Tudor" },
      "Mary_Ann_Tudor_ahnentafel.txt",
    ],
    [{ given_name: "", surname: "Tudor" }, "Tudor_ahnentafel.txt"],
    [{}, "Unknown_ahnentafel.txt"],
  ])("names the file from %o", async (entry, expected) => {
    expect(await exportFilename(entry)).toBe(expected);
  });
});

describe("Ahnentafel page: summary counts ancestors, not the subject (#1119)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  const person = (number: number, generation: number) => ({
    number,
    generation,
    id: `p-${number}`,
    given_name: `G${number}`,
    surname: "Hanover",
    relationship: "",
  });

  async function summary(report: Partial<apiModule.AhnentafelResponse>) {
    vi.mocked(apiModule.api.getAhnentafel).mockResolvedValue({
      subject: { id: "p-1", given_name: "G1", surname: "Hanover" },
      ...report,
    } as apiModule.AhnentafelResponse);
    resetRouter("/ahnentafel/p-1", { id: "p-1" });
    const { container } = render(AhnentafelPage);
    await waitFor(() =>
      expect(container.querySelector(".report-meta")).not.toBeNull(),
    );
    return (container.querySelector(".report-meta")?.textContent ?? "")
      .replace(/\s+/g, " ")
      .trim();
  }

  it("reads one generation in the singular, with the API's ancestor counts", async () => {
    // The API leaves the subject out of known_ancestors and ancestor_slots;
    // known_ancestors equals the pedigree's total_ancestors (Go API test).
    expect(
      await summary({
        entries: [person(1, 0), person(2, 1), person(3, 1)],
        generations: 1,
        total_count: 3,
        known_count: 3,
        ancestor_slots: 2,
        known_ancestors: 2,
      }),
    ).toBe("1 generation - 2 of 2 ancestors known");
  });

  it("reads several generations in the plural", async () => {
    expect(
      await summary({
        entries: [person(1, 0), person(2, 1), person(5, 2)],
        generations: 2,
        total_count: 7,
        known_count: 3,
        ancestor_slots: 6,
        known_ancestors: 2,
      }),
    ).toBe("2 generations - 2 of 6 ancestors known");
  });
});
