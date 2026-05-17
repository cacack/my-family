/**
 * Shared styling tokens for form controls.
 *
 * The native <select> element doesn't have a shadcn-svelte primitive in this
 * project, but we want it to look consistent with `<Input>`. This class string
 * mirrors the shadcn Input signature so a native <select class={nativeSelectClass}>
 * lines up visually with surrounding Input/Textarea controls.
 */
export const nativeSelectClass =
	'flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50';
