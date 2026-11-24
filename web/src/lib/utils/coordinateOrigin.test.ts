import { describe, it, expect } from "vitest";
import {
  coordinateOriginText,
  coordinateSourceLabel,
  coordinateSourcesText,
} from "./coordinateOrigin.js";

describe("coordinateOrigin", () => {
  it("labels each source", () => {
    expect(coordinateSourceLabel("imported")).toBe("Imported");
    expect(coordinateSourceLabel("manual")).toBe("Entered by hand");
    expect(coordinateSourceLabel("copied")).toBe("Copied from another place");
    expect(coordinateSourceLabel("geocoded")).toBe("Geocoded");
  });

  it("names a geocoded origin's provider and precision", () => {
    expect(
      coordinateOriginText({
        source: "geocoded",
        provider: "Nominatim",
        precision: "full",
      }),
    ).toBe("Geocoded by Nominatim, full match");
    expect(coordinateOriginText({ source: "manual" })).toBe("Entered by hand");
  });

  it("joins a map location's sources", () => {
    expect(coordinateSourcesText(["imported", "manual"])).toBe(
      "Imported, Entered by hand",
    );
  });
});
