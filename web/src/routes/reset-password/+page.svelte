<script lang="ts">
	/** A password reset link's page; the root layout shows it whoever is signed in. */
	import { page } from '$app/state';
	import ResetPasswordForm from '#lib/components/auth/ResetPasswordForm.svelte';
	import { tokenFromHash } from '#lib/utils/accountLinks.js';
</script>

<ResetPasswordForm token={tokenFromHash(page.url.hash)} />
