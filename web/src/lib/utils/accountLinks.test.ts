import { describe, it, expect } from 'vitest';
import { inviteLink, resetLink, tokenFromHash } from './accountLinks';

describe('account links', () => {
	it('puts the token in the fragment of the page that takes it', () => {
		expect(inviteLink('abc-_1', 'https://tree.example')).toBe('https://tree.example/join#token=abc-_1');
		expect(resetLink('abc', 'https://tree.example')).toBe('https://tree.example/reset-password#token=abc');
	});

	it('reads the token back from the fragment', () => {
		expect(tokenFromHash('#token=abc-_1')).toBe('abc-_1');
		expect(tokenFromHash(new URL(inviteLink('a b', 'http://x')).hash)).toBe('a b');
		expect(tokenFromHash('')).toBe('');
		expect(tokenFromHash('#other=1')).toBe('');
	});
});
