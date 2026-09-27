/**
 * Shown in place of the Restore tab on person and family pages while a
 * research branch is active (#824).
 *
 * Rollback is mainline-only (ADR-005): its version and deleted checks read the
 * mainline, so restoring on a branch would rewrite the mainline. The pages
 * withdraw the Restore tab and the rollback dialog on a branch, and the API
 * refuses a branch-scoped rollback or restore-point request as a backstop.
 */
export const ROLLBACK_MAINLINE_ONLY =
	'Restore points and rollback work on the mainline only, so they are not offered while a research branch is active. The change log shows this branch’s view: its own edits and the mainline history it inherits.';
