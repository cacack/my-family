/**
 * Split a typed full name: the last word is the surname, the rest the given
 * name. A surname written between slashes, as GEDCOM does (`Mary /van Buren/`,
 * `/Tudor/`), is taken whole, so a surname can stand alone (#1709).
 */
export function parseName(fullName: string): { givenName: string; surname: string } {
	const trimmed = fullName.trim().replace(/\s+/g, ' ');
	if (!trimmed) {
		return { givenName: '', surname: '' };
	}
	const slashed = /^(.*?)\s*\/([^/]*)\/?$/.exec(trimmed);
	if (slashed) {
		return { givenName: slashed[1], surname: slashed[2].trim() };
	}
	const lastSpace = trimmed.lastIndexOf(' ');
	if (lastSpace === -1) {
		return { givenName: trimmed, surname: '' };
	}
	return {
		givenName: trimmed.substring(0, lastSpace),
		surname: trimmed.substring(lastSpace + 1)
	};
}
