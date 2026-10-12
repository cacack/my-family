import type { ConflictField } from '$lib/components/SaveConflict.svelte';
import { rebaseEdits, sameValue } from './rebaseEdits';

/**
 * A whole-record edit form, as the conflict flow sees it: the saved version
 * the form was opened on, the form's values, and how to put a newer version
 * under the form.
 */
export interface SaveConflictForm<R, F extends Record<string, unknown>> {
	/** The saved version the form was opened on (and saves against). */
	base: () => R | null;
	/** The form's values for a saved version. */
	formFrom: (saved: R) => F;
	/** The form's values as typed. */
	form: () => F;
	/** Open the form on `latest`, with these values in it. */
	adopt: (latest: R, form: F) => void;
	/** Save the form against the version it is open on. */
	save: () => void;
}

/** A row of the comparison: a form field, and how to show its value. */
export interface ConflictRow<F> {
	key: keyof F & string;
	label: string;
	/** Display text for a value; by default lists are joined and empty is blank. */
	format?: (value: F[keyof F]) => string;
}

function displayValue(value: unknown): string {
	if (Array.isArray(value)) return value.join(', ');
	return value == null ? '' : String(value);
}

/**
 * What an edit form does with a save refused because the record changed since
 * it was opened (#899, #939). The form keeps the user's edits; `latest` holds
 * the record as saved now, for `SaveConflict` to compare with. "Save my edits"
 * rebases the edits onto it, so another writer's changes to the fields the user
 * didn't touch survive, and saves; "Use the latest version" drops the edits.
 */
export class SaveConflictFlow<R, F extends Record<string, unknown>> {
	/** The latest saved version, after a save refused because it changed. */
	latest: R | null = $state(null);

	constructor(private readonly target: SaveConflictForm<R, F>) {}

	/**
	 * Fetch the latest version to compare with, leaving the form as typed.
	 * False when it can't be re-read, for the page to show the refusal as it came.
	 */
	async load(fetchLatest: () => Promise<R>): Promise<boolean> {
		try {
			this.latest = await fetchLatest();
			return true;
		} catch {
			return false;
		}
	}

	clear() {
		this.latest = null;
	}

	/** Whether the user changed a field from the version the form was opened on. */
	edited(key: keyof F): boolean {
		const base = this.target.base();
		return !!base && !sameValue(this.target.form()[key], this.target.formFrom(base)[key]);
	}

	/**
	 * The fields the user edited, beside the latest saved version, as display
	 * text. Fields left alone are not listed, as saving takes the latest for them.
	 */
	fields(rows: ConflictRow<F>[]): ConflictField[] {
		if (!this.latest) return [];
		const mine = this.target.form();
		const theirs = this.target.formFrom(this.latest);
		return rows
			.filter((row) => this.edited(row.key))
			.map(({ key, label, format = displayValue }) => ({
				label,
				mine: format(mine[key]),
				latest: format(theirs[key])
			}));
	}

	/** Save the edits over the latest version, keeping its other changes. */
	keepMine = () => {
		const base = this.target.base();
		const latest = this.latest;
		if (!base || !latest) return;
		const { formFrom, form } = this.target;
		this.target.adopt(latest, rebaseEdits(form(), formFrom(base), formFrom(latest)));
		this.target.save();
	};

	/** Drop the edits for the latest version. */
	useLatest = () => {
		const latest = this.latest;
		if (!latest) return;
		this.latest = null;
		this.target.adopt(latest, this.target.formFrom(latest));
	};
}
