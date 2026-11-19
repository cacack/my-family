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

/** The accessible names of the badges a person carries, living first. */
export function personBadgeNames(person: PrivacyMarked): string[] {
	const names: string[] = [];
	if (person.living) names.push(LIVING_BADGE_NAME);
	if (person.private) names.push(privateBadgeName());
	return names;
}
