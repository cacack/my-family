/**
 * The pages the whole-app guards visit: every top-level route plus a seeded
 * detail page of each kind. Shared so the layout and accessibility guards
 * cover the same ground - a new page added here is checked by both.
 */
import type { SeedData } from './seed';

export const STATIC_ROUTES = [
	'/',
	'/persons',
	'/persons/add',
	'/persons/quick',
	'/families',
	'/families/add',
	'/browse/surnames',
	'/browse/places',
	'/browse/cemeteries',
	'/browse/brick-walls',
	'/browse/citation-templates',
	'/repositories',
	'/sources',
	'/evidence',
	'/history',
	'/branches',
	'/snapshots',
	'/map',
	'/analytics',
	'/quality',
	'/relationship',
	'/import',
	'/search'
];

// Detail routes need seeded ids, and the seed can only be read once global
// setup has run - so callers build them inside a test, not at collection time.
export const DETAIL_ROUTES: ((s: SeedData) => string)[] = [
	(s) => `/persons/${s.switcher.person.id}`,
	(s) => `/pedigree/${s.switcher.person.id}`,
	(s) => `/descendancy/${s.switcher.person.id}`,
	(s) => `/ahnentafel/${s.switcher.person.id}`,
	(s) => `/families/${s.merge.familyId}`,
	(s) => `/families/${s.merge.familyId}/group-sheet`,
	(s) => `/branches/${s.switcher.branchId}`,
	(s) => `/snapshots/compare?from=${s.snapshots.beforeId}&to=${s.snapshots.afterId}`
];
