/**
 * Whether two form values are the same: equal primitives, or arrays (a list of
 * linked ids) with the same items in the same order. A form holds its own copy
 * of an array, so identity would call every list edited.
 */
export function sameValue(a: unknown, b: unknown): boolean {
	if (Array.isArray(a) && Array.isArray(b)) {
		return a.length === b.length && a.every((item, i) => item === b[i]);
	}
	return a === b;
}

/**
 * Edits made in a form opened on `base`, carried over to `latest`: a field the
 * user changed keeps the user's value, and a field they left alone takes the
 * latest one (#899). Without this, saving after a 409 would write the form's
 * stale copy of untouched fields back over another writer's changes.
 */
export function rebaseEdits<T extends Record<string, unknown>>(form: T, base: T, latest: T): T {
	const rebased = { ...form };
	for (const key of Object.keys(form) as (keyof T)[]) {
		if (sameValue(form[key], base[key])) rebased[key] = latest[key];
	}
	return rebased;
}
