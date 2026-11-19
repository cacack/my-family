import type { AuthAuditEntry } from '#lib/api/client.js';

/** How the audit log names each action. */
export const AUTH_ACTION_LABELS: Record<AuthAuditEntry['action'], string> = {
	sign_in: 'Signed in',
	sign_in_failed: 'Sign-in failed',
	user_created: 'Owner created',
	invite_created: 'Invite created',
	invite_accepted: 'Invite accepted',
	role_changed: 'Role changed',
	user_disabled: 'Account disabled',
	user_enabled: 'Account re-enabled',
	password_reset_issued: 'Reset link issued',
	password_reset: 'Password reset',
	password_changed: 'Password changed',
	anonymous_access_changed: 'Visitor access changed'
};

/** Who did it: the actor's name, the CLI, or no one signed in (a failed sign-in, setup). */
export function auditActor(entry: AuthAuditEntry): string {
	if (entry.actor_name) return entry.actor_name;
	if (entry.client_ip === 'cli') return 'Command line';
	return entry.actor_id ? 'A removed user' : 'Not signed in';
}
