import { auth, type Role } from '#lib/stores/access.svelte.js';

/** Sign a test user in with `role`; the setup file resets to open mode before each test. */
export function signInAs(role: Role): void {
	auth.phase = 'signed-in';
	auth.account = { id: `user-${role}`, username: role, display_name: role, role };
}
