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
		// Parse date-only strings as UTC to avoid timezone day-shift
		if (/^\d{4}-\d{2}-\d{2}$/.test(dateStr)) {
			const [year, month, day] = dateStr.split('-').map(Number);
			const date = new Date(Date.UTC(year, month - 1, day));
			if (isNaN(date.getTime())) return dateStr;
			return date.toLocaleDateString();
		}
		const date = new Date(dateStr);
		if (isNaN(date.getTime())) return dateStr;
		return date.toLocaleDateString();
	} catch {
		return dateStr;
	}
}

/** Convert a date input value (YYYY-MM-DD) to RFC3339 for the API. */
export function toRFC3339(dateStr: string): string {
	if (dateStr.includes('T')) return dateStr;
	return dateStr + 'T00:00:00Z';
}

/**
 * Props for rendering a research log's outcome as a <Badge>.
 *
 * Single source of truth so EvidencePanel, the evidence hub list, and any
 * future surface share the same colour and label per outcome value.
 */
export interface BadgeProps {
	variant: 'default' | 'secondary' | 'destructive' | 'outline';
	class: string;
	label: string;
}

export function outcomeBadgeProps(outcome: string): BadgeProps {
	switch (outcome) {
		case 'found':
			return {
				variant: 'secondary',
				class:
					'border-green-200 bg-green-50 text-green-700 dark:border-green-800 dark:bg-green-950 dark:text-green-400',
				label: 'Found'
			};
		case 'not_found':
			return { variant: 'destructive', class: '', label: 'Not Found' };
		case 'inconclusive':
			return {
				variant: 'secondary',
				class:
					'border-yellow-200 bg-yellow-50 text-yellow-700 dark:border-yellow-800 dark:bg-yellow-950 dark:text-yellow-400',
				label: 'Inconclusive'
			};
		default:
			return {
				variant: 'secondary',
				class:
					'border-yellow-200 bg-yellow-50 text-yellow-700 dark:border-yellow-800 dark:bg-yellow-950 dark:text-yellow-400',
				label: outcome.replace(/_/g, ' ')
			};
	}
}

/**
 * Props for rendering a conflict's status as a <Badge>.
 *
 * Single source of truth so EvidencePanel, the hub list, and the conflict
 * detail page render the same colours for the same status.
 */
export function conflictBadgeProps(status: string): BadgeProps {
	if (status === 'open') {
		return { variant: 'destructive', class: '', label: 'Open' };
	}
	return {
		variant: 'secondary',
		class: 'border-green-200 bg-green-50 text-green-700',
		label: 'Resolved'
	};
}

/** Tailwind classes for an EvidencePanel conflict row's wrapper (open vs resolved). */
export function conflictRowClass(status: string): string {
	return status === 'open'
		? 'border-amber-200 bg-amber-50'
		: 'border-slate-200 bg-slate-50';
}
