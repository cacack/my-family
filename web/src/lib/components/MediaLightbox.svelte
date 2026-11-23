<script lang="ts">
	import { can } from '#lib/stores/access.svelte.js';
	import { activeBranch } from '#lib/stores/activeBranch.svelte.js';
	import { api, isReferenceOnly, isStaleVersion, type Media } from '#lib/api/client.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as Dialog from '#lib/components/ui/dialog/index.js';

	interface Props {
		open: boolean;
		media: Media;
		allMedia: Media[];
		onClose: () => void;
		/** Called with an item this lightbox changed, such as its private mark. */
		onUpdate?: (media: Media) => void;
	}

	let { open = $bindable(), media, allMedia, onClose, onUpdate }: Props = $props();

	let marking = $state(false);
	let markError: string | null = $state(null);
	/** The mark a stale save asked for, which Try again resends. */
	let retryMark: boolean | null = $state(null);

	/**
	 * Mark the item private, or unmark it (ADR-008). A stale version loads
	 * the latest one and offers to resend the same mark: the toggle alone
	 * would flip it when a grouped copy failed after this item was saved,
	 * and resending is what brings the group back in step.
	 */
	async function setPrivate(mark: boolean) {
		// Required: `aria-disabled` does not block clicks. Not `disabled`
		// while saving, which would drop the button's focus.
		if (marking) return;
		const target = currentMedia;
		marking = true;
		markError = null;
		retryMark = null;
		try {
			onUpdate?.(await api.updateMedia(target.id, { private: mark, version: target.version }));
		} catch (e) {
			if (isStaleVersion(e)) {
				markError = 'This item was changed elsewhere and has been reloaded.';
				retryMark = mark;
				try {
					onUpdate?.(await api.getMedia(target.id));
				} catch {
					// Keep the notice; Try again reports any failure.
				}
			} else {
				markError = (e as { message?: string }).message || 'Failed to update the item';
			}
		} finally {
			marking = false;
		}
	}

	let attaching = $state(false);
	let attachError: string | null = $state(null);
	let attachInput: HTMLInputElement | undefined = $state();

	/**
	 * Upload the file of a reference-only item into the item itself (#1504),
	 * which keeps its title, GEDCOM link and private mark.
	 */
	async function attachFile(event: Event) {
		const input = event.currentTarget as HTMLInputElement;
		const file = input.files?.[0];
		input.value = '';
		if (!file || attaching) return;
		const target = currentMedia;
		attaching = true;
		attachError = null;
		try {
			onUpdate?.(await api.attachMediaContent(target.id, file));
		} catch (e) {
			attachError = (e as { message?: string }).message || 'Failed to attach the file';
		} finally {
			attaching = false;
		}
	}

	let currentIndex = $derived(allMedia.findIndex((m) => m.id === media.id));
	let currentMedia = $derived(allMedia[currentIndex] || media);
	let isImage = $derived(currentMedia.mime_type.startsWith('image/'));
	let isPdf = $derived(currentMedia.mime_type === 'application/pdf');
	let referenceOnly = $derived(isReferenceOnly(currentMedia));
	let referencePaths = $derived(
		(currentMedia.files ?? []).map((f) => f.path).filter((p): p is string => !!p)
	);

	function goToPrevious() {
		if (currentIndex > 0) {
			navigateTo(currentIndex - 1);
		}
	}

	function goToNext() {
		if (currentIndex < allMedia.length - 1) {
			navigateTo(currentIndex + 1);
		}
	}

	function navigateTo(index: number) {
		const newMedia = allMedia[index];
		if (newMedia) {
			markError = null;
			retryMark = null;
			attachError = null;
			Object.assign(media, newMedia);
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (!open) return;
		if (e.key === 'ArrowLeft') {
			goToPrevious();
		} else if (e.key === 'ArrowRight') {
			goToNext();
		}
	}

	function formatFileSize(bytes: number): string {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
	}

	function formatDate(dateStr: string): string {
		return new Date(dateStr).toLocaleDateString(undefined, {
			year: 'numeric',
			month: 'long',
			day: 'numeric'
		});
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<Dialog.Root bind:open onOpenChange={(isOpen) => { if (!isOpen) onClose(); }}>
	<!--
		A phone gets the whole screen, opaque, so the page behind cannot be read (#1555).
		Width is capped from md up: an sm: cap from the base dialog would beat a max-md: one.
	-->
	<Dialog.Content
		showCloseButton={false}
		class="max-w-none sm:max-w-none md:max-w-[95vw] h-[90vh] grid-rows-[minmax(0,1fr)] p-0 bg-black/95 border-none max-md:inset-0 max-md:h-dvh max-md:translate-none max-md:rounded-none max-md:bg-black"
	>
		<div class="sr-only">
			<Dialog.Header>
				<Dialog.Title>Media lightbox - {currentMedia.title}</Dialog.Title>
			</Dialog.Header>
		</div>

		<!-- Close button -->
		<Button variant="ghost" size="icon" class="absolute top-4 right-4 z-10 rounded-full text-white hover:bg-white/20" onclick={onClose} aria-label="Close lightbox">
			<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="size-6">
				<line x1="18" y1="6" x2="6" y2="18" />
				<line x1="6" y1="6" x2="18" y2="18" />
			</svg>
		</Button>

		<!-- Navigation buttons -->
		{#if allMedia.length > 1 && currentIndex > 0}
			<Button variant="ghost" size="icon" class="nav-prev absolute top-1/2 left-4 z-10 -translate-y-1/2 rounded-full text-white hover:bg-white/20 h-12 w-12" onclick={goToPrevious} aria-label="Previous media">
				<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="size-6">
					<polyline points="15 18 9 12 15 6" />
				</svg>
			</Button>
		{/if}

		{#if allMedia.length > 1 && currentIndex < allMedia.length - 1}
			<Button variant="ghost" size="icon" class="nav-next absolute top-1/2 right-4 z-10 -translate-y-1/2 rounded-full text-white hover:bg-white/20 h-12 w-12" onclick={goToNext} aria-label="Next media">
				<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="size-6">
					<polyline points="9 18 15 12 9 6" />
				</svg>
			</Button>
		{/if}

		<div class="lightbox-content">
			<div class="media-container">
				{#if referenceOnly}
					<div class="document-preview">
						<svg class="document-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true">
							<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
							<polyline points="14 2 14 8 20 8" />
						</svg>
						{#each referencePaths as path, i (i)}
							<p class="document-name">{path}</p>
						{/each}
						<p class="reference-note">The file is not stored here; this item only records where it was.</p>
						<!-- Mainline only: the attach operation takes no branch scope. -->
						{#if can('editor') && !activeBranch.id}
							<div class="attach-action">
								<input
									bind:this={attachInput}
									type="file"
									accept="image/jpeg,image/png,image/gif,image/webp,image/tiff,application/pdf"
									class="sr-only"
									tabindex="-1"
									aria-hidden="true"
									onchange={attachFile}
								/>
								<Button
									variant="outline"
									class="border-white/40 bg-transparent text-white hover:bg-white/20 hover:text-white"
									aria-disabled={attaching}
									onclick={() => !attaching && attachInput?.click()}
									>{attaching ? 'Attaching…' : 'Attach file'}</Button
								>
								{#if attachError}
									<p class="mark-error" role="alert">{attachError}</p>
								{/if}
							</div>
						{/if}
					</div>
				{:else if isImage}
					<img
						src={api.getMediaContentUrl(currentMedia.id)}
						alt={currentMedia.title}
						class="media-image"
					/>
				{:else if isPdf}
					<div class="document-preview">
						<svg class="document-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
							<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
							<polyline points="14 2 14 8 20 8" />
							<line x1="16" y1="13" x2="8" y2="13" />
							<line x1="16" y1="17" x2="8" y2="17" />
							<polyline points="10 9 9 9 8 9" />
						</svg>
						<p class="document-name">{currentMedia.filename}</p>
						<Button
							href={api.getMediaContentUrl(currentMedia.id)}
							target="_blank"
							rel="noopener noreferrer"
						>
							<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
								<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
								<polyline points="7 10 12 15 17 10" />
								<line x1="12" y1="15" x2="12" y2="3" />
							</svg>
							View PDF
						</Button>
					</div>
				{:else}
					<div class="document-preview">
						<svg class="document-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
							<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
							<polyline points="14 2 14 8 20 8" />
						</svg>
						<p class="document-name">{currentMedia.filename}</p>
						<Button
							href={api.getMediaContentUrl(currentMedia.id)}
							download={currentMedia.filename}
						>
							<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
								<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
								<polyline points="7 10 12 15 17 10" />
								<line x1="12" y1="15" x2="12" y2="3" />
							</svg>
							Download File
						</Button>
					</div>
				{/if}
			</div>

			<div class="metadata-panel">
				<h3>{currentMedia.title}</h3>

				<!-- Under the title, so it is in view on a phone, where the panel scrolls (#1550). -->
				{#if can('editor')}
					<div class="privacy-action">
						<Button
							variant="outline"
							size="sm"
							class="border-white/40 bg-transparent text-white hover:bg-white/20 hover:text-white"
							aria-disabled={marking}
							onclick={() => setPrivate(!currentMedia.private)}
							>{currentMedia.private ? 'Unmark private' : 'Mark private'}</Button
						>
						{#if markError}
							<p class="mark-error" role="alert">{markError}</p>
						{/if}
						{#if retryMark !== null}
							<Button
								variant="outline"
								size="sm"
								class="border-white/40 bg-transparent text-white hover:bg-white/20 hover:text-white"
								aria-disabled={marking}
								onclick={() => retryMark !== null && setPrivate(retryMark)}
								>{retryMark ? 'Try again: mark private' : 'Try again: unmark private'}</Button
							>
						{/if}
					</div>
				{/if}

				{#if currentMedia.description}
					<p class="description">{currentMedia.description}</p>
				{/if}

				<dl class="metadata-list">
					{#if referenceOnly}
						<dt>File</dt>
						<dd>Not stored (reference only)</dd>
					{:else}
						<dt>Filename</dt>
						<dd>{currentMedia.filename}</dd>

						<dt>Size</dt>
						<dd>{formatFileSize(currentMedia.file_size)}</dd>
					{/if}

					{#if currentMedia.media_type}
						<dt>Type</dt>
						<dd class="capitalize">{currentMedia.media_type}</dd>
					{/if}

					<dt>{referenceOnly ? 'Imported' : 'Uploaded'}</dt>
					<dd>{formatDate(currentMedia.created_at)}</dd>

					<dt>Privacy</dt>
					<dd>{currentMedia.private ? 'Private: viewers and visitors do not see it' : 'Not private'}</dd>
				</dl>

				{#if allMedia.length > 1}
					<div class="media-counter">
						{currentIndex + 1} of {allMedia.length}
					</div>
				{/if}
			</div>
		</div>
	</Dialog.Content>
</Dialog.Root>

<style>
.lightbox-content {
		display: flex;
		gap: 2rem;
		width: 100%;
		height: 100%;
		padding: 2rem;
	}

	.media-container {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		min-width: 0;
	}

	.media-image {
		max-width: 100%;
		max-height: calc(90vh - 4rem);
		object-fit: contain;
		border-radius: 4px;
	}

	.document-preview {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		padding: 3rem;
		background: rgba(255, 255, 255, 0.05);
		border-radius: 8px;
		text-align: center;
	}

	.document-icon {
		width: 4rem;
		height: 4rem;
		color: #94a3b8;
		margin-bottom: 1rem;
	}

	.document-name {
		color: white;
		font-size: 1rem;
		margin: 0 0 1.5rem;
		word-break: break-word;
	}

	.reference-note {
		color: white;
		font-size: 0.875rem;
		margin: 0;
		max-width: 28rem;
	}

	.metadata-panel {
		width: 280px;
		flex-shrink: 0;
		background: rgba(255, 255, 255, 0.05);
		border-radius: 8px;
		padding: 1.5rem;
		color: white;
		overflow-y: auto;
	}

	.metadata-panel h3 {
		margin: 0 0 0.75rem;
		/* Not break-word: anywhere also narrows the dialog's sizing to fit (#1555). */
		overflow-wrap: anywhere;
		font-size: 1.125rem;
		font-weight: 600;
	}

	.description {
		margin: 0 0 1.5rem;
		color: rgb(255 255 255 / 0.8);
		font-size: 0.875rem;
		line-height: 1.5;
	}

	.metadata-list {
		margin: 0;
		display: grid;
		grid-template-columns: auto 1fr;
		gap: 0.5rem 1rem;
	}

	.metadata-list dt {
		color: rgb(255 255 255 / 0.65);
		font-size: 0.8125rem;
	}

	.metadata-list dd {
		margin: 0;
		font-size: 0.875rem;
		word-break: break-word;
	}

	.capitalize {
		text-transform: capitalize;
	}

	.privacy-action {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 0.5rem;
		margin-bottom: 1rem;
	}

	.attach-action {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.5rem;
		margin-top: 1.5rem;
	}

	.mark-error {
		margin: 0;
		color: #fecaca;
		font-size: 0.8125rem;
	}

	.media-counter {
		margin-top: 1.5rem;
		padding-top: 1rem;
		border-top: 1px solid rgba(255, 255, 255, 0.1);
		text-align: center;
		color: rgb(255 255 255 / 0.65);
		font-size: 0.875rem;
	}

	@media (max-width: 768px) {
		.lightbox-content {
			flex-direction: column;
			gap: 1rem;
			/* The top keeps a tall image clear of the close button (#1555). */
			padding: 4rem 1rem 1rem;
		}

		/* Shrink a tall image to the space left, rather than push the panel off the dialog. */
		.media-container {
			min-height: 0;
		}

		.media-image {
			max-height: 100%;
		}

		.metadata-panel {
			width: 100%;
			max-height: 200px;
		}

		.document-preview {
			padding: 1.5rem 1rem;
		}
	}
</style>
