import { tick } from 'svelte';
import type { ApiError } from '#lib/api/client.js';

/** A save refused for one of a form's date fields, shown beside that field (#1112). */
export interface DateFieldError<F extends string> {
	field: F;
	/** The server's message without its "invalid input: <field>: " prefix. */
	message: string;
	/** The text can't be read as a date at all, so it can be kept as text instead. */
	unreadable: boolean;
}

/**
 * The date field a save was refused for, when it is one of `fields`; null for
 * any other error. The server names the field in `details.field`.
 */
export function dateFieldError<F extends string>(error: unknown, fields: readonly F[]): DateFieldError<F> | null {
	const apiError = error as ApiError | undefined;
	const field = fields.find((f) => f === apiError?.details?.field);
	if (!field) return null;
	const message = apiError?.message ?? '';
	const prefix = `invalid input: ${field}: `;
	return {
		field,
		message: message.startsWith(prefix) ? message.slice(prefix.length) : message,
		unreadable: apiError?.details?.unreadable === true
	};
}

/**
 * The date field a save was refused for because its text can't be read as a
 * date at all, when it is one of `fields`; null for any other error (#978).
 * Such a date can still be saved as a GEDCOM date phrase (see `asDatePhrase`).
 * A date that reads but doesn't exist, like "31 FEB 1900", is not one.
 */
export function unreadableDateField<F extends string>(error: unknown, fields: readonly F[]): F | null {
	const dateError = dateFieldError(error, fields);
	return dateError?.unreadable ? dateError.field : null;
}

/** The id of the message shown beside a refused date field. */
export function dateErrorId(field: string): string {
	return `${field}-error`;
}

/** The ARIA attributes tying a refused date input to its message; none for any other input. */
export function dateErrorAttrs(
	error: DateFieldError<string> | null,
	field: string
): { 'aria-invalid'?: 'true'; 'aria-describedby'?: string } {
	return error?.field === field ? { 'aria-invalid': 'true', 'aria-describedby': dateErrorId(field) } : {};
}

/** Focus the first field marked invalid in `form`, once a failed submit has rendered. */
export async function focusFirstInvalid(form: HTMLFormElement | null | undefined): Promise<void> {
	await tick();
	form?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus();
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

/**
 * `note` while `stored` is still what it says was saved, else undefined: a
 * note carried in history state comes back on Back/Forward even after the
 * date has been edited since (#996).
 */
export function stillStored(note: SavedAs | undefined, stored: string): SavedAs | undefined {
	return note?.stored === stored ? note : undefined;
}

/** The line a detail page shows under a date saved in a different form, if any. */
export function savedAsText(note: SavedAs | undefined): string | undefined {
	return note && `Saved as ${note.stored} from “${note.typed}”`;
}
