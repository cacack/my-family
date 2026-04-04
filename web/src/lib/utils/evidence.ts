/**
 * Shared utility functions for evidence analysis UI.
 */

/** Format a fact_type enum value for display (e.g., "person_birth" → "Person Birth"). */
export function formatFactType(factType: string): string {
	return factType
		.replace(/_/g, ' ')
		.replace(/\b\w/g, (c) => c.toUpperCase());
}

/** Format a fact_type with prefix stripped (e.g., "person_birth" → "Birth"). Use in contexts where the subject type is already clear. */
export function formatFactTypeShort(factType: string): string {
	return factType
		.replace(/^(person_|family_)/, '')
		.replace(/_/g, ' ')
		.replace(/\b\w/g, (c) => c.toUpperCase());
}

/** Derive the route prefix from a fact_type (person_* → "persons", family_* → "families"). */
export function subjectRoute(factType: string): string {
	return factType.startsWith('family_') ? 'families' : 'persons';
}

/** Format a date string for display, with fallback to the raw string. */
export function formatDate(dateStr: string): string {
	try {
		return new Date(dateStr).toLocaleDateString();
	} catch {
		return dateStr;
	}
}

/** Convert a date input value (YYYY-MM-DD) to RFC3339 for the API. */
export function toRFC3339(dateStr: string): string {
	if (dateStr.includes('T')) return dateStr;
	return new Date(dateStr + 'T00:00:00').toISOString();
}
