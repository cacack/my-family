export function parseName(fullName: string): { givenName: string; surname: string } {
	const trimmed = fullName.trim().replace(/\s+/g, ' ');
	if (!trimmed) {
		return { givenName: '', surname: '' };
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
