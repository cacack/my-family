import { describe, it, expect, vi, beforeEach } from "vitest";
import { fireEvent, render, screen } from "@testing-library/svelte";
import ChangeHistory from "./ChangeHistory.svelte";
import * as apiModule from "$lib/api/client";

vi.mock("$lib/api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof apiModule>();
  return {
    ...actual,
    api: {
      getPersonHistory: vi.fn(),
      getFamilyHistory: vi.fn(),
      getSourceHistory: vi.fn(),
      getGlobalHistory: vi.fn(),
    },
  };
});

const PERSON_ID = "11111111-1111-1111-1111-111111111111";

function entry(
  overrides: Partial<apiModule.ChangeEntry>,
): apiModule.ChangeEntry {
  return {
    id: crypto.randomUUID(),
    timestamp: "2026-01-15T10:30:00Z",
    entity_type: "person",
    entity_id: PERSON_ID,
    entity_name: "Ada Lovelace",
    action: "updated",
    ...overrides,
  };
}

describe("ChangeHistory origin labels (#824)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("labels branch-scoped entries as the branch’s own or inherited from the mainline", async () => {
    vi.mocked(apiModule.api.getPersonHistory).mockResolvedValue({
      items: [
        entry({ action: "created", origin: "main" }),
        entry({ origin: "branch" }),
      ],
      total: 2,
      limit: 20,
      offset: 0,
      has_more: false,
    });

    render(ChangeHistory, { entityType: "person", entityId: PERSON_ID });

    expect(await screen.findByText("Mainline")).toBeDefined();
    expect(screen.getByText("This branch")).toBeDefined();
  });

  it("shows no origin label on mainline history", async () => {
    vi.mocked(apiModule.api.getFamilyHistory).mockResolvedValue({
      items: [entry({ entity_type: "family", action: "created" })],
      total: 1,
      limit: 20,
      offset: 0,
      has_more: false,
    });

    render(ChangeHistory, { entityType: "family", entityId: PERSON_ID });

    expect(await screen.findByText("created")).toBeDefined();
    expect(screen.queryByText("Mainline")).toBeNull();
    expect(screen.queryByText("This branch")).toBeNull();
  });
});

describe("ChangeHistory global log (#739)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders every entity type by label and name, linking sub-records to their owner", async () => {
    const NOTE_ID = "22222222-2222-2222-2222-222222222222";
    const CITATION_ID = "33333333-3333-3333-3333-333333333333";
    const SOURCE_ID = "44444444-4444-4444-4444-444444444444";
    const ANALYSIS_ID = "55555555-5555-5555-5555-555555555555";
    vi.mocked(apiModule.api.getGlobalHistory).mockResolvedValue({
      items: [
        entry({
          id: "n",
          entity_type: "note",
          entity_id: NOTE_ID,
          entity_name: "A note excerpt",
          action: "updated",
        }),
        entry({
          id: "c",
          entity_type: "citation",
          entity_id: CITATION_ID,
          entity_name: "1850 Census (Birth)",
          parent_entity_type: "source",
          parent_entity_id: SOURCE_ID,
        }),
        entry({
          id: "a",
          entity_type: "evidence_analysis",
          entity_id: ANALYSIS_ID,
          entity_name: "Birth: Born 1815",
          action: "created",
        }),
        entry({
          id: "m",
          action: "merged",
          changes: { merged_person: { new_value: "Ada Byron" } },
        }),
      ],
      total: 4,
      limit: 20,
      offset: 0,
      has_more: false,
    });

    render(ChangeHistory);

    expect(await screen.findByText("A note excerpt")).toBeDefined();
    expect(
      screen.getByText("Note", { selector: ".entity-type" }),
    ).toBeDefined();
    expect(screen.queryByRole("link", { name: "A note excerpt" })).toBeNull();
    // No page to link to is not the same as deleted: a live note is not struck through.
    expect(
      screen.getByText("A note excerpt").classList.contains("deleted"),
    ).toBe(false);
    expect(
      screen
        .getByRole("link", { name: "1850 Census (Birth)" })
        .getAttribute("href"),
    ).toBe(`/sources/${SOURCE_ID}`);
    expect(
      screen.getByText("Evidence analysis", { selector: ".entity-type" }),
    ).toBeDefined();
    expect(
      screen
        .getByRole("link", { name: "Birth: Born 1815" })
        .getAttribute("href"),
    ).toBe(`/evidence/analyses/${ANALYSIS_ID}`);
    // A person merge offers its field changes like an update does.
    expect(screen.getByText("merged")).toBeDefined();
    expect(screen.getByRole("button", { name: /Show changes/ })).toBeDefined();
  });

  it("offers every entity type in the filter", async () => {
    vi.mocked(apiModule.api.getGlobalHistory).mockResolvedValue({
      items: [],
      total: 0,
      limit: 20,
      offset: 0,
      has_more: false,
    });

    render(ChangeHistory);

    const select = await screen.findByLabelText("Entity Type");
    const values = [...select.querySelectorAll("option")].map((o) =>
      o.getAttribute("value"),
    );
    expect(values).toContain("life_event");
    expect(values).toContain("research_log");
    expect(values).toContain("branch");
    expect(values).toHaveLength(18);
  });
});

describe("ChangeHistory merge provenance and branch lifecycle (#832)", () => {
  const BRANCH_ID = "66666666-6666-6666-6666-666666666666";

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("marks a merged change with the branch and note it came from", async () => {
    vi.mocked(apiModule.api.getPersonHistory).mockResolvedValue({
      items: [
        entry({ action: "created" }),
        entry({
          merged_from: {
            branch_id: BRANCH_ID,
            branch_name: "Byron theory",
            note: "Baptism register settles it",
            merged_at: "2026-02-01T09:00:00Z",
            original_timestamp: "2026-01-20T09:00:00Z",
          },
        }),
      ],
      total: 2,
      limit: 20,
      offset: 0,
      has_more: false,
    });

    render(ChangeHistory, { entityType: "person", entityId: PERSON_ID });

    const chip = await screen.findByRole("link", {
      name: "via merge of Byron theory: Baptism register settles it",
    });
    expect(chip.getAttribute("href")).toBe(`/branches/${BRANCH_ID}`);
    expect(chip.getAttribute("title")).toMatch(
      /Made on the branch Jan 20, 2026/,
    );
    expect(screen.getAllByTestId("merged-from")).toHaveLength(1);
  });

  it("names a merge without a note by its branch alone", async () => {
    vi.mocked(apiModule.api.getPersonHistory).mockResolvedValue({
      items: [
        entry({
          merged_from: {
            branch_id: BRANCH_ID,
            branch_name: "Byron theory",
            merged_at: "2026-02-01T09:00:00Z",
            original_timestamp: "2026-01-20T09:00:00Z",
          },
        }),
      ],
      total: 1,
      limit: 20,
      offset: 0,
      has_more: false,
    });

    render(ChangeHistory, { entityType: "person", entityId: PERSON_ID });

    expect(
      await screen.findByRole("link", { name: "via merge of Byron theory" }),
    ).toBeDefined();
  });

  it("renders the branch lifecycle, linking to the branch even once archived", async () => {
    vi.mocked(apiModule.api.getGlobalHistory).mockResolvedValue({
      items: [
        entry({
          id: "c",
          entity_type: "branch",
          entity_id: BRANCH_ID,
          entity_name: "Byron theory",
          action: "created",
        }),
        entry({
          id: "m",
          entity_type: "branch",
          entity_id: BRANCH_ID,
          entity_name: "Byron theory",
          action: "merged",
          changes: { merge_note: { new_value: "Baptism register settles it" } },
        }),
        entry({
          id: "d",
          entity_type: "branch",
          entity_id: "77777777-7777-7777-7777-777777777777",
          entity_name: "Dead end",
          action: "deleted",
        }),
        entry({
          id: "x",
          entity_type: "branch",
          entity_id: "88888888-8888-8888-8888-888888888888",
          entity_name: "Wrong parish",
          action: "deleted",
          changes: {
            outcome: { new_value: "disproved" },
            close_reason: { new_value: "The will names other heirs" },
          },
        }),
      ],
      total: 4,
      limit: 20,
      offset: 0,
      has_more: false,
    });

    render(ChangeHistory);

    const links = await screen.findAllByRole("link", { name: "Byron theory" });
    expect(links.map((l) => l.getAttribute("href"))).toEqual([
      `/branches/${BRANCH_ID}`,
      `/branches/${BRANCH_ID}`,
    ]);
    expect(
      screen.getAllByText("Research branch", { selector: ".entity-type" }),
    ).toHaveLength(4);
    expect(screen.getByText("archived")).toBeDefined();
    expect(screen.getByText("closed")).toBeDefined();
    expect(screen.getByText(/The will names other heirs/)).toBeDefined();
    expect(
      screen.getByRole("link", { name: "Dead end" }).getAttribute("href"),
    ).toBe("/branches/77777777-7777-7777-7777-777777777777");
    expect(screen.getByRole("button", { name: /Show changes/ })).toBeDefined();
  });
});

describe("ChangeHistory failed loads (#899)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("offers Retry when the history fails to load", async () => {
    vi.mocked(apiModule.api.getGlobalHistory)
      .mockRejectedValueOnce({ message: "Database unavailable" })
      .mockResolvedValueOnce({ items: [entry({})], total: 1, limit: 20, offset: 0, has_more: false });
    render(ChangeHistory);

    expect((await screen.findByRole("alert")).textContent).toContain("Database unavailable");
    await fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("Ada Lovelace")).toBeTruthy();
  });

  it("keeps the entries shown when Load more fails", async () => {
    vi.mocked(apiModule.api.getGlobalHistory)
      .mockResolvedValueOnce({ items: [entry({})], total: 2, limit: 1, offset: 0, has_more: true })
      .mockRejectedValueOnce({ message: "Timed out" });
    render(ChangeHistory);

    await fireEvent.click(await screen.findByRole("button", { name: "Load more" }));

    expect((await screen.findByRole("alert")).textContent).toContain("Timed out");
    expect(screen.getByText("Ada Lovelace")).toBeTruthy();
  });
});
