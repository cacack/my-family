<script lang="ts">
	import { api, isConflictError, type Media } from '$lib/api/client';
	import MediaUpload from './MediaUpload.svelte';
	import MediaLightbox from './MediaLightbox.svelte';
	import ConflictError from './ConflictError.svelte';

	interface Props {
		personId: string;
		onMediaAdded?: (media: Media) => void;
	}

	let { personId, onMediaAdded }: Props = $props();

	let mediaItems: Media[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);
	let lightboxMedia: Media | null = $state(null);
	let deletingId: string | null = $state(null);

	// Conflict retry state
	let conflictError = $state(false);
	let retryAction: (() => Promise<void>) | null = $state(null);
	let retrying = $state(false);

	async function loadMedia() {
		loading = true;
		error = null;
		try {
			const response = await api.listPersonMedia(personId);
			mediaItems = response.items;
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load media';
		} finally {
			loading = false;
		}
	}

	function handleUpload(media: Media) {
		mediaItems = [media, ...mediaItems];
		onMediaAdded?.(media);
	}

	function openLightbox(media: Media) {
		lightboxMedia = media;
	}

	function closeLightbox() {
		lightboxMedia = null;
	}

	async function handleRetry() {
		if (!retryAction) return;
		retrying = true;
		conflictError = false;
		error = null;
		try {
			await retryAction();
		} catch (e) {
			if (isConflictError(e)) {
				conflictError = true;
			} else {
				error = (e as { message?: string }).message || 'Operation failed';
			}
		} finally {
			retrying = false;
		}
	}

	async function deleteMedia(media: Media, e: MouseEvent) {
		e.stopPropagation();
		if (!confirm(`Delete "${media.title}"? This cannot be undone.`)) return;

		deletingId = media.id;
		try {
			await api.deleteMedia(media.id, media.version);
			mediaItems = mediaItems.filter((m) => m.id !== media.id);
			conflictError = false;
		} catch (e) {
			if (isConflictError(e)) {
				conflictError = true;
				retryAction = async () => {
					deletingId = media.id;
					try {
						const fresh = await api.getMedia(media.id);
						await api.deleteMedia(media.id, fresh.version);
						mediaItems = mediaItems.filter((m) => m.id !== media.id);
						conflictError = false;
					} finally {
						deletingId = null;
					}
				};
			} else {
				error = (e as { message?: string }).message || 'Failed to delete media';
			}
		} finally {
			deletingId = null;
		}
	}

	$effect(() => {
		if (personId) {
			loadMedia();
		}
	});
</script>

<div class="media-gallery">
	<MediaUpload {personId} onUpload={handleUpload} />

	{#if conflictError}
		<ConflictError onRetry={handleRetry} {retrying} />
	{/if}

	{#if loading}
		<div class="loading-state" role="status" aria-live="polite">
			<span class="spinner" aria-hidden="true"></span>
			Loading media...
		</div>
	{:else if error}
		<div class="error-state" role="alert">
			<p>{error}</p>
			<button class="btn" onclick={() => loadMedia()}>Retry</button>
		</div>
	{:else if mediaItems.length === 0}
		<div class="empty-state">
			<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
				<rect x="3" y="3" width="18" height="18" rx="2" ry="2" />
				<circle cx="8.5" cy="8.5" r="1.5" />
				<polyline points="21 15 16 10 5 21" />
			</svg>
			<p>No media yet. Upload photos and documents.</p>
		</div>
	{:else}
		<div class="gallery-grid" role="list" aria-label="Media gallery">
			{#each mediaItems as media (media.id)}
				<div
					class="thumbnail-card"
					class:deleting={deletingId === media.id}
					role="listitem"
				>
					<button
						type="button"
						class="thumbnail-button"
						onclick={() => openLightbox(media)}
						aria-label="View {media.title}"
					>
						{#if media.has_thumbnail}
							<img
								src={api.getMediaThumbnailUrl(media.id)}
								alt=""
								class="thumbnail"
							/>
						{:else}
							<div class="thumbnail-placeholder" aria-hidden="true">
								<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
									<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
									<polyline points="14 2 14 8 20 8" />
								</svg>
							</div>
						{/if}

						<div class="thumbnail-overlay" aria-hidden="true">
							<span class="thumbnail-title">{media.title}</span>
						</div>
					</button>

					<button
						class="delete-btn"
						onclick={(e) => deleteMedia(media, e)}
						disabled={deletingId === media.id}
						aria-label="Delete {media.title}"
					>
						{#if deletingId === media.id}
							<span class="spinner-small"></span>
						{:else}
							<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
								<polyline points="3 6 5 6 21 6" />
								<path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
							</svg>
						{/if}
					</button>
				</div>
			{/each}
		</div>
	{/if}
</div>

{#if lightboxMedia}
	<MediaLightbox
		media={lightboxMedia}
		allMedia={mediaItems}
		onClose={closeLightbox}
	/>
{/if}

<style>
	.media-gallery {
		/* MediaGallery container */
		display: block;
	}

	.gallery-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
		gap: 1rem;
	}

	.thumbnail-card {
		position: relative;
		aspect-ratio: 1;
		border-radius: 8px;
		overflow: hidden;
		background: #f1f5f9;
		transition: transform 0.2s, box-shadow 0.2s;
	}

	.thumbnail-card:hover {
		transform: translateY(-2px);
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
	}

	.thumbnail-button {
		display: block;
		width: 100%;
		height: 100%;
		padding: 0;
		border: none;
		background: none;
		cursor: pointer;
		position: relative;
	}

	.thumbnail-button:focus-visible {
		outline: 2px solid #3b82f6;
		outline-offset: 2px;
	}

	.thumbnail-card.deleting {
		opacity: 0.5;
		pointer-events: none;
	}

	.thumbnail {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.thumbnail-placeholder {
		width: 100%;
		height: 100%;
		display: flex;
		align-items: center;
		justify-content: center;
		background: #e2e8f0;
	}

	.thumbnail-placeholder svg {
		width: 3rem;
		height: 3rem;
		color: #94a3b8;
	}

	.thumbnail-overlay {
		position: absolute;
		inset: 0;
		background: linear-gradient(to top, rgba(0, 0, 0, 0.7) 0%, transparent 50%);
		display: flex;
		align-items: flex-end;
		padding: 0.75rem;
		opacity: 0;
		transition: opacity 0.2s;
	}

	.thumbnail-card:hover .thumbnail-overlay {
		opacity: 1;
	}

	.thumbnail-title {
		color: white;
		font-size: 0.8125rem;
		font-weight: 500;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.delete-btn {
		position: absolute;
		top: 0.5rem;
		right: 0.5rem;
		width: 2rem;
		height: 2rem;
		border: none;
		background: rgba(255, 255, 255, 0.9);
		border-radius: 50%;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		opacity: 0;
		transition: opacity 0.2s, background 0.2s;
	}

	.thumbnail-card:hover .delete-btn {
		opacity: 1;
	}

	.delete-btn:hover {
		background: #fee2e2;
	}

	.delete-btn svg {
		width: 1rem;
		height: 1rem;
		color: #dc2626;
	}

	.delete-btn:disabled {
		cursor: not-allowed;
	}

	.loading-state,
	.error-state,
	.empty-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		padding: 3rem;
		text-align: center;
		color: #64748b;
	}

	.empty-state svg {
		width: 3rem;
		height: 3rem;
		color: #cbd5e1;
		margin-bottom: 1rem;
	}

	.empty-state p,
	.loading-state,
	.error-state p {
		margin: 0;
		font-size: 0.875rem;
	}

	.error-state p {
		color: #dc2626;
		margin-bottom: 1rem;
	}

	.spinner {
		width: 1.25rem;
		height: 1.25rem;
		border: 2px solid #e2e8f0;
		border-top-color: #3b82f6;
		border-radius: 50%;
		animation: spin 0.6s linear infinite;
		margin-bottom: 0.75rem;
	}

	.spinner-small {
		width: 0.875rem;
		height: 0.875rem;
		border: 2px solid #e2e8f0;
		border-top-color: #3b82f6;
		border-radius: 50%;
		animation: spin 0.6s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	.btn {
		padding: 0.5rem 1rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
		cursor: pointer;
		color: #475569;
	}

	.btn:hover {
		background: #f1f5f9;
	}

	@media (max-width: 640px) {
		.gallery-grid {
			grid-template-columns: repeat(2, 1fr);
			gap: 0.75rem;
		}
	}
</style>
