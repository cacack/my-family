/**
 * The main navigation, grouped by what the user is trying to do. The header
 * renders it as inline links and menus on wide screens and as one sectioned
 * menu on narrow ones, so both always offer the same destinations.
 */

import type { Role } from '#lib/stores/access.svelte.js';

/**
 * `match` lists the path prefixes that count as being in a link's section.
 * `minRole` hides the link from a role below it (ADR-007). `signedIn` hides it
 * from a visitor, whom the server refuses the change log (ADR-008 §4).
 */
export type NavLink = { href: string; label: string; match?: string[]; minRole?: Role; signedIn?: boolean };
export type NavGroup = { label: string; links: NavLink[] };
export type NavEntry = NavLink | NavGroup;

export const mainNav: NavEntry[] = [
	// The chart pages are reached from a person, so they belong to People.
	{ href: '/persons', label: 'People', match: ['/persons', '/pedigree', '/descendancy', '/ahnentafel'] },
	{ href: '/families', label: 'Families' },
	{
		label: 'Browse',
		links: [
			{ href: '/browse/surnames', label: 'By Surname' },
			{ href: '/browse/places', label: 'By Place' },
			{ href: '/browse/cemeteries', label: 'By Cemetery' },
			{ href: '/map', label: 'Map' },
			{ href: '/relationship', label: 'Relationship' }
		]
	},
	{
		label: 'Research',
		links: [
			{ href: '/sources', label: 'Sources' },
			{ href: '/evidence', label: 'Evidence' },
			{ href: '/repositories', label: 'Repositories' },
			{ href: '/browse/citation-templates', label: 'Citation Templates' },
			{ href: '/browse/brick-walls', label: 'Brick Walls' },
			{ href: '/quality', label: 'Quality' },
			{ href: '/analytics', label: 'Completeness' }
		]
	},
	// A branch's page is its comparison, part of the change log.
	{
		label: 'History',
		links: [
			{ href: '/history', label: 'Change History', signedIn: true },
			{ href: '/branches', label: 'Branches', signedIn: true },
			{ href: '/snapshots', label: 'Snapshots', signedIn: true }
		]
	},
	// The page also exports, which an editor may do; importing needs the owner.
	{ href: '/import', label: 'Import', minRole: 'editor' }
];

/**
 * The navigation without the links `can` refuses, or that need a signed-in
 * user when `visitor`, and without groups left empty.
 */
export function navFor(can: (role: Role) => boolean, visitor = false): NavEntry[] {
	const allowed = (link: NavLink) => (!link.minRole || can(link.minRole)) && !(visitor && link.signedIn);
	return mainNav.flatMap((entry): NavEntry[] => {
		if (!isGroup(entry)) return allowed(entry) ? [entry] : [];
		const links = entry.links.filter(allowed);
		return links.length ? [{ ...entry, links }] : [];
	});
}

export function isGroup(entry: NavEntry): entry is NavGroup {
	return 'links' in entry;
}

/** Matches whole path segments, so /persons/123 is in People but /personsx is not. */
export function isLinkActive(link: NavLink, pathname: string): boolean {
	return (link.match ?? [link.href]).some(
		(prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`)
	);
}

export function isEntryActive(entry: NavEntry, pathname: string): boolean {
	return isGroup(entry)
		? entry.links.some((link) => isLinkActive(link, pathname))
		: isLinkActive(entry, pathname);
}
