/**
 * Words for where a coordinate pair came from (#1563), so a geocoder's guess
 * is never read as a location taken from a source.
 */
import type { CoordinateOrigin, CoordinateSource } from "#lib/api/client.js";

const SOURCE_LABELS: Record<CoordinateSource, string> = {
  imported: "Imported",
  manual: "Entered by hand",
  copied: "Copied from another place",
  geocoded: "Geocoded",
};

/** The label of one source: "Imported", "Geocoded". */
export function coordinateSourceLabel(source: CoordinateSource): string {
  return SOURCE_LABELS[source] ?? source;
}

/**
 * One pair's origin in words. A geocoded pair names its provider and how much
 * of the place name it matched: "Geocoded by Nominatim, partial match".
 */
export function coordinateOriginText(origin: CoordinateOrigin): string {
  const label = coordinateSourceLabel(origin.source);
  if (origin.source !== "geocoded") return label;
  const by = origin.provider ? ` by ${origin.provider}` : "";
  const match = origin.precision ? `, ${origin.precision} match` : "";
  return `${label}${by}${match}`;
}

/** The distinct sources of the pairs a map location groups, in words. */
export function coordinateSourcesText(sources: CoordinateSource[]): string {
  return sources.map(coordinateSourceLabel).join(", ");
}
