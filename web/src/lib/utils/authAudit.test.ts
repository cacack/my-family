import { describe, it, expect } from 'vitest';
import { auditActor } from './authAudit';
import type { AuthAuditEntry } from '#lib/api/client.js';

const base: AuthAuditEntry = { at: '2026-01-01T00:00:00Z', action: 'sign_in', username: 'ada', detail: '', client_ip: '192.0.2.1' };

describe('auditActor', () => {
	it('names the actor, the CLI, or no one', () => {
		expect(auditActor({ ...base, actor_id: 'x', actor_name: 'Ada' })).toBe('Ada');
		expect(auditActor({ ...base, client_ip: 'cli' })).toBe('Command line');
		expect(auditActor({ ...base, actor_id: 'x' })).toBe('A removed user');
		expect(auditActor({ ...base, action: 'sign_in_failed' })).toBe('Not signed in');
	});
});
