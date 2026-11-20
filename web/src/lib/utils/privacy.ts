/**
 * The privacy marks of a person or record (ADR-008), shared by the HTML
 * badges and the chart markers so both name them the same way.
 */

/**
 * What carries the marks: a person's living verdict, any record's private
 * flag, and the redacted mark of the placeholder shown instead of a person
 * the reader may not see (ADR-008 §3).
 */
export interface PrivacyMarked {
	living?: boolean;
	private?: boolean;
	redacted?: boolean;
}

/** The accessible name of the living badge. */
export const LIVING_BADGE_NAME = 'Living person';

/** The accessible name of the placeholder's badge. */
export const REDACTED_BADGE_NAME = 'Details hidden';

/** The accessible name of the private badge on a record of kind `noun`. */
export function privateBadgeName(noun = 'person'): string {
	return `Private ${noun}`;
}

/** One badge a person carries: which mark it is, and its accessible name. */
export interface PersonBadge {
	kind: 'living' | 'private' | 'redacted';
	name: string;
}

/**
 * The badges a person carries, living first. A placeholder carries only its
 * own: its other marks describe nothing the reader may see.
 */
export function personBadges(person: PrivacyMarked): PersonBadge[] {
	if (person.redacted) return [{ kind: 'redacted', name: REDACTED_BADGE_NAME }];
	const badges: PersonBadge[] = [];
	if (person.living) badges.push({ kind: 'living', name: LIVING_BADGE_NAME });
	if (person.private) badges.push({ kind: 'private', name: privateBadgeName() });
	return badges;
}

/** The accessible names of the badges a person carries, living first. */
export function personBadgeNames(person: PrivacyMarked): string[] {
	return personBadges(person).map((badge) => badge.name);
}
