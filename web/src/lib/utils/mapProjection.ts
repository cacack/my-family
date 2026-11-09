/**
 * The geographic map's initial projection, fitted to the family's locations.
 */
import * as d3 from 'd3';

/** Tall enough to tap markers on a phone; capped on wide screens. */
export function mapHeight(width: number): number {
	return Math.max(320, Math.min(width * 0.55, 600));
}

/**
 * A Natural Earth projection for a `width` x `height` map that shows every point
 * at least `margin` pixels inside the edges.
 *
 * It starts at the scale that fills the map (on narrow screens the world is then
 * wider than the map) and only shrinks when the points would not fit. It never
 * zooms in past that, so the whole family stays in its world context. Then it
 * centres on the points, panning no further than needed to keep them inside and,
 * where the points allow, without showing past the world's edge.
 */
export function fitMapProjection(
	width: number,
	height: number,
	points: [number, number][],
	margin: number
): d3.GeoProjection {
	const fill = Math.max(width / 5.5, height / 2.8);
	// At translate [0, 0] a projected position is linear in the scale
	const unit = d3.geoNaturalEarth1().scale(1).translate([0, 0]);
	const projected = points.map((p) => unit(p) ?? [0, 0]);
	const [minX = 0, maxX = 0] = d3.extent(projected, (p) => p[0]);
	const [minY = 0, maxY = 0] = d3.extent(projected, (p) => p[1]);

	const [roomX, roomY] = [width - 2 * margin, height - 2 * margin];
	let scale = fill;
	// A map with no room for the margins (not laid out yet) keeps the filling scale
	if (maxX > minX && roomX > 0) scale = Math.min(scale, roomX / (maxX - minX));
	if (maxY > minY && roomY > 0) scale = Math.min(scale, roomY / (maxY - minY));

	const halfWidth = unit([180, 0])![0] * scale;
	const halfHeight = -unit([0, 90])![1] * scale;
	const tx = centre(width, halfWidth, minX * scale, maxX * scale, margin);
	const ty = centre(height, halfHeight, minY * scale, maxY * scale, margin);
	return d3.geoNaturalEarth1().scale(scale).translate([tx, ty]);
}

/**
 * The offset along one axis that centres the points [lo, hi] in a map of `size`,
 * clamped to keep them `margin` inside, and the world (half size `half`) either
 * covering the map or wholly inside it, whichever it can.
 */
function centre(size: number, half: number, lo: number, hi: number, margin: number): number {
	const [pointsMin, pointsMax] = [margin - lo, size - margin - hi];
	const [worldMin, worldMax] = [Math.min(half, size - half), Math.max(half, size - half)];
	const min = Math.max(pointsMin, worldMin);
	const max = Math.min(pointsMax, worldMax);
	const ideal = size / 2 - (lo + hi) / 2;
	return min <= max ? clamp(ideal, min, max) : clamp(ideal, pointsMin, pointsMax);
}

function clamp(value: number, min: number, max: number): number {
	return Math.min(max, Math.max(min, value));
}
