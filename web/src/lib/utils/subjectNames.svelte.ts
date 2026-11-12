import { SvelteMap } from 'svelte/reactivity';
import { api, formatFamilyName, formatPersonName } from '#lib/api/client.js';

/** The person or family an evidence record is about. */
export interface Subject {
	type: 'person' | 'family';
	id: string;
}

/** The page a subject links to. */
export function subjectHref({ type, id }: Subject): string {
	return `/${type === 'family' ? 'families' : 'persons'}/${id}`;
}

/** A subject's display name, or null when it can't be read (deleted, say). */
async function readName({ type, id }: Subject): Promise<string | null> {
	try {
		return type === 'family'
			? formatFamilyName(await api.getFamily(id))
			: formatPersonName(await api.getPerson(id));
	} catch {
		return null;
	}
}

const key = ({ type, id }: Subject) => `${type}:${id}`;

/**
 * The names of the subjects a page shows (#1108): evidence records store only
 * a subject id, and a page should show a name, not a UUID. A page creates one
 * and calls `load` with the subjects it is about to show; each is read once,
 * and `label` re-renders when its name arrives.
 */
export class SubjectNames {
	#names = new SvelteMap<string, string | null>();
	#pending = new Set<string>();

	/** Reads the names not already known or being read. */
	async load(subjects: Subject[]): Promise<void> {
		const unread = new Map<string, Subject>();
		for (const subject of subjects) {
			const k = key(subject);
			if (!this.#names.has(k) && !this.#pending.has(k)) unread.set(k, subject);
		}
		await Promise.all(
			[...unread].map(async ([k, subject]) => {
				this.#pending.add(k);
				const name = await readName(subject);
				this.#pending.delete(k);
				this.#names.set(k, name);
			})
		);
	}

	/** The subject's name, once read; undefined before then or when it can't be read. */
	name(subject: Subject): string | undefined {
		return this.#names.get(key(subject)) ?? undefined;
	}

	/** The subject's name for display: `…` while it loads, `Unknown person` when it can't be read. */
	label(subject: Subject): string {
		const k = key(subject);
		if (!this.#names.has(k)) return '…';
		return this.#names.get(k) ?? `Unknown ${subject.type}`;
	}
}
