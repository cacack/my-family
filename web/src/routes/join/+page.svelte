<script lang="ts">
	/** An invite link's page; the root layout shows it whoever is signed in. */
	import { page } from '$app/state';
	import JoinForm from '#lib/components/auth/JoinForm.svelte';
	import { tokenFromHash } from '#lib/utils/accountLinks.js';
</script>

<JoinForm token={tokenFromHash(page.url.hash)} />
