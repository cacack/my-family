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
 *
 * Keys in one `together` group are one value, such as a latitude and longitude:
 * editing any of them keeps all of them, so a rebase never joins one writer's
 * latitude to another's longitude (#1033).
 */
export function rebaseEdits<T extends Record<string, unknown>>(
	form: T,
	base: T,
	latest: T,
	together: (keyof T)[][] = []
): T {
	const rebased = { ...form };
	for (const key of Object.keys(form) as (keyof T)[]) {
		if (!editedWith(key, form, base, together)) rebased[key] = latest[key];
	}
	return rebased;
}

/** Whether `key`, or a key edited together with it, differs from `base`. */
export function editedWith<T extends Record<string, unknown>>(
	key: keyof T,
	form: T,
	base: T,
	together: (keyof T)[][] = []
): boolean {
	const group = together.find((keys) => keys.includes(key)) ?? [key];
	return group.some((k) => !sameValue(form[k], base[k]));
}
