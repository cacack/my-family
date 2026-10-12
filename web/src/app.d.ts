// See https://svelte.dev/docs/kit/types#app.d.ts
// for information about these interfaces
declare global {
	namespace App {
		// interface Error {}
		// interface Locals {}
		// interface PageData {}
		interface PageState {
			/** A one-off confirmation shown by the page navigated to, e.g. after a delete. */
			notice?: string;
			/** The dates a create stored in another form than typed, for the new record's page (#996). */
			savedAs?: Partial<Record<string, import('$lib/utils/dateInput').SavedAs>>;
		}
		// interface Platform {}
	}
}

export {};
