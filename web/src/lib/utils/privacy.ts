/**
 * The privacy marks of a person or record (ADR-008), shared by the HTML
 * badges and the chart markers so both name them the same way.
 */

/** What carries the marks: a person's living verdict and any record's private flag. */
export interface PrivacyMarked {
	living?: boolean;
	private?: boolean;
}

/** The accessible name of the living badge. */
export const LIVING_BADGE_NAME = 'Living person';

/** The accessible name of the private badge on a record of kind `noun`. */
export function privateBadgeName(noun = 'person'): string {
	return `Private ${noun}`;
}

/** One badge a person carries: which mark it is, and its accessible name. */
export interface PersonBadge {
	kind: 'living' | 'private';
	name: string;
}

/** The badges a person carries, living first. */
export function personBadges(person: PrivacyMarked): PersonBadge[] {
	const badges: PersonBadge[] = [];
	if (person.living) badges.push({ kind: 'living', name: LIVING_BADGE_NAME });
	if (person.private) badges.push({ kind: 'private', name: privateBadgeName() });
	return badges;
}

/** The accessible names of the badges a person carries, living first. */
export function personBadgeNames(person: PrivacyMarked): string[] {
	return personBadges(person).map((badge) => badge.name);
}
