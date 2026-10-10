/**
 * The main navigation, grouped by what the user is trying to do. The header
 * renders it as inline links and menus on wide screens and as one sectioned
 * menu on narrow ones, so both always offer the same destinations.
 */

/** `match` lists the path prefixes that count as being in a link's section. */
export type NavLink = { href: string; label: string; match?: string[] };
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
			{ href: '/analytics', label: 'Analytics' }
		]
	},
	{
		label: 'History',
		links: [
			{ href: '/history', label: 'Change History' },
			{ href: '/branches', label: 'Branches' },
			{ href: '/snapshots', label: 'Snapshots' }
		]
	},
	{ href: '/import', label: 'Import' }
];

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
