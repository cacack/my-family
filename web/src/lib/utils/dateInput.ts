import type { ApiError } from '$lib/api/client';

/**
 * The date field a save was refused for because its text can't be read as a
 * date at all, when it is one of `fields`; null for any other error (#978).
 * Such a date can still be saved as a GEDCOM date phrase (see `asDatePhrase`).
 * A date that reads but doesn't exist, like "31 FEB 1900", is not one.
 */
export function unreadableDateField<F extends string>(error: unknown, fields: readonly F[]): F | null {
	const details = (error as ApiError | undefined)?.details;
	if (details?.unreadable !== true) return null;
	return fields.find((field) => field === details.field) ?? null;
}

/** A typed date as a GEDCOM date phrase, which is kept as text. */
export function asDatePhrase(value: string): string {
	return `(${value.trim()})`;
}

/** A date the server stored in a different form from the one typed. */
export interface SavedAs {
	typed: string;
	stored: string;
}

/**
 * The date fields the server stored in a different form from the one typed,
 * such as "about 1850" saved as "ABT 1850" (#978), so the form can say so.
 * Surrounding whitespace alone is not a difference.
 */
export function savedAsNotes<F extends string>(
	typed: Record<F, string>,
	stored: Record<NoInfer<F>, string>
): Partial<Record<F, SavedAs>> {
	const notes: Partial<Record<F, SavedAs>> = {};
	for (const field of Object.keys(typed) as F[]) {
		const value = typed[field].trim();
		if (value && value !== stored[field]) notes[field] = { typed: value, stored: stored[field] };
	}
	return notes;
}
