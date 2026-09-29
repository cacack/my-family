import { describe, it, expect } from "vitest";
import {
  CHANGE_ENTITY_TYPES,
  changeActionLabel,
  changeEntryLink,
  entityTypeLabel,
  mergedFromLabel,
  unnamedEntityLabel,
} from "./changeEntries";

const ID = "11111111-1111-1111-1111-111111111111";
const PARENT = "22222222-2222-2222-2222-222222222222";

describe("changeEntryLink", () => {
  it.each([
    ["person", `/persons/${ID}`],
    ["family", `/families/${ID}`],
    ["source", `/sources/${ID}`],
    ["repository", `/repositories/${ID}`],
    ["evidence_analysis", `/evidence/analyses/${ID}`],
    ["evidence_conflict", `/evidence/conflicts/${ID}`],
    ["research_log", `/evidence/research-logs/${ID}`],
    ["proof_summary", `/evidence/proof-summaries/${ID}`],
    ["branch", `/branches/${ID}`],
  ] as const)("links a %s to its own page", (entityType, href) => {
    expect(
      changeEntryLink({
        entity_type: entityType,
        entity_id: ID,
        action: "updated",
      }),
    ).toBe(href);
  });

  it("links a sub-record to the page that presents it, even once deleted", () => {
    const lifeEvent = {
      entity_type: "life_event" as const,
      entity_id: ID,
      parent_entity_type: "family",
      parent_entity_id: PARENT,
    };
    expect(changeEntryLink({ ...lifeEvent, action: "updated" })).toBe(
      `/families/${PARENT}`,
    );
    expect(changeEntryLink({ ...lifeEvent, action: "deleted" })).toBe(
      `/families/${PARENT}`,
    );
  });

  it("does not link a deleted entity or one without any page", () => {
    expect(
      changeEntryLink({
        entity_type: "person",
        entity_id: ID,
        action: "deleted",
      }),
    ).toBeNull();
    expect(
      changeEntryLink({
        entity_type: "note",
        entity_id: ID,
        action: "created",
      }),
    ).toBeNull();
    expect(
      changeEntryLink({
        entity_type: "submitter",
        entity_id: ID,
        action: "updated",
      }),
    ).toBeNull();
  });
});

describe("entityTypeLabel", () => {
  it("labels every entity type", () => {
    expect(CHANGE_ENTITY_TYPES).toHaveLength(17);
    for (const type of CHANGE_ENTITY_TYPES) {
      expect(entityTypeLabel(type)).not.toBe(type);
    }
    expect(entityTypeLabel("lds_ordinance")).toBe("LDS ordinance");
  });

  it("falls back to a readable form of an unrecognised type", () => {
    expect(entityTypeLabel("some_thing")).toBe("Some thing");
    expect(entityTypeLabel("")).toBe("Entity");
  });
});

describe("unnamedEntityLabel", () => {
  it('names the entity type rather than saying "entity"', () => {
    expect(unnamedEntityLabel("evidence_analysis")).toBe(
      "Unnamed evidence analysis",
    );
    expect(unnamedEntityLabel("life_event")).toBe("Unnamed life event");
    expect(unnamedEntityLabel("lds_ordinance")).toBe("Unnamed LDS ordinance");
    expect(unnamedEntityLabel("")).toBe("Unnamed entity");
  });
});

describe("branch lifecycle and merge provenance (#832)", () => {
  it("links an archived branch to its page, and calls its deletion archiving", () => {
    const archived = {
      entity_type: "branch" as const,
      entity_id: ID,
      action: "deleted" as const,
    };
    expect(changeEntryLink(archived)).toBe(`/branches/${ID}`);
    expect(changeActionLabel(archived)).toBe("archived");
    expect(
      changeActionLabel({
        ...archived,
        changes: { outcome: { new_value: "disproved" } },
      }),
    ).toBe("closed");
    expect(changeActionLabel({ entity_type: "branch", action: "merged" })).toBe(
      "merged",
    );
    expect(
      changeActionLabel({ entity_type: "person", action: "deleted" }),
    ).toBe("deleted");
  });

  it('reads a merge origin as "via merge of <branch>: <note>"', () => {
    const origin = {
      branch_id: ID,
      branch_name: "Byron theory",
      merged_at: "2026-02-01T09:00:00Z",
      original_timestamp: "2026-01-20T09:00:00Z",
    };
    expect(mergedFromLabel(origin)).toBe("via merge of Byron theory");
    expect(mergedFromLabel({ ...origin, note: "  register  " })).toBe(
      "via merge of Byron theory: register",
    );
    expect(mergedFromLabel({ ...origin, branch_name: "" })).toBe(
      "via merge of a research branch",
    );
  });
});
