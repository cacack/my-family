<script lang="ts">
	import { api, type Media } from '$lib/api/client';

	interface Props {
		media: Media;
		allMedia: Media[];
		onClose: () => void;
	}

	let { media, allMedia, onClose }: Props = $props();

	let currentIndex = $derived(allMedia.findIndex((m) => m.id === media.id));
	let currentMedia = $derived(allMedia[currentIndex] || media);
	let isImage = $derived(currentMedia.mime_type.startsWith('image/'));
	let isPdf = $derived(currentMedia.mime_type === 'application/pdf');

	function goToPrevious() {
		if (currentIndex > 0) {
			const prevMedia = allMedia[currentIndex - 1];
			// Update the media prop by calling onClose and reopening
			// Actually, we need to update the parent - for now, navigate via index
			navigateTo(currentIndex - 1);
		}
	}

	function goToNext() {
		if (currentIndex < allMedia.length - 1) {
			navigateTo(currentIndex + 1);
		}
	}

	function navigateTo(index: number) {
		// We'll update the currentIndex by using a local state override
		const newMedia = allMedia[index];
		if (newMedia) {
			// Replace media reference for the component
			Object.assign(media, newMedia);
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			onClose();
		} else if (e.key === 'ArrowLeft') {
			goToPrevious();
		} else if (e.key === 'ArrowRight') {
			goToNext();
		}
	}

	function handleOverlayClick(e: MouseEvent) {
		// Close if clicking the overlay background, not the content
		if (e.target === e.currentTarget) {
			onClose();
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

<svelte:window on:keydown={handleKeydown} />

<!-- svelte-ignore a11y_no_static_element_interactions -->
<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="lightbox-overlay" onclick={handleOverlayClick}>
	<button class="close-btn" onclick={onClose} aria-label="Close lightbox">
		<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
			<line x1="18" y1="6" x2="6" y2="18" />
			<line x1="6" y1="6" x2="18" y2="18" />
		</svg>
	</button>

	{#if allMedia.length > 1 && currentIndex > 0}
		<button class="nav-btn nav-prev" onclick={goToPrevious} aria-label="Previous media">
			<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
				<polyline points="15 18 9 12 15 6" />
			</svg>
		</button>
	{/if}

	{#if allMedia.length > 1 && currentIndex < allMedia.length - 1}
		<button class="nav-btn nav-next" onclick={goToNext} aria-label="Next media">
			<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
				<polyline points="9 18 15 12 9 6" />
			</svg>
		</button>
	{/if}

	<div class="lightbox-content">
		<div class="media-container">
			{#if isImage}
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
					<a
						href={api.getMediaContentUrl(currentMedia.id)}
						target="_blank"
						rel="noopener noreferrer"
						class="btn btn-primary"
					>
						<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
							<polyline points="7 10 12 15 17 10" />
							<line x1="12" y1="15" x2="12" y2="3" />
						</svg>
						View PDF
					</a>
				</div>
			{:else}
				<div class="document-preview">
					<svg class="document-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
						<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
						<polyline points="14 2 14 8 20 8" />
					</svg>
					<p class="document-name">{currentMedia.filename}</p>
					<a
						href={api.getMediaContentUrl(currentMedia.id)}
						download={currentMedia.filename}
						class="btn btn-primary"
					>
						<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
							<polyline points="7 10 12 15 17 10" />
							<line x1="12" y1="15" x2="12" y2="3" />
						</svg>
						Download File
					</a>
				</div>
			{/if}
		</div>

		<div class="metadata-panel">
			<h3>{currentMedia.title}</h3>

			{#if currentMedia.description}
				<p class="description">{currentMedia.description}</p>
			{/if}

			<dl class="metadata-list">
				<dt>Filename</dt>
				<dd>{currentMedia.filename}</dd>

				<dt>Size</dt>
				<dd>{formatFileSize(currentMedia.file_size)}</dd>

				{#if currentMedia.media_type}
					<dt>Type</dt>
					<dd class="capitalize">{currentMedia.media_type}</dd>
				{/if}

				<dt>Uploaded</dt>
				<dd>{formatDate(currentMedia.created_at)}</dd>
			</dl>

			{#if allMedia.length > 1}
				<div class="media-counter">
					{currentIndex + 1} of {allMedia.length}
				</div>
			{/if}
		</div>
	</div>
</div>

<style>
	.lightbox-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.9);
		z-index: 1000;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 2rem;
	}

	.close-btn {
		position: absolute;
		top: 1rem;
		right: 1rem;
		width: 2.5rem;
		height: 2.5rem;
		border: none;
		background: rgba(255, 255, 255, 0.1);
		border-radius: 50%;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: background 0.2s;
		z-index: 10;
	}

	.close-btn svg {
		width: 1.5rem;
		height: 1.5rem;
		color: white;
	}

	.close-btn:hover {
		background: rgba(255, 255, 255, 0.2);
	}

	.nav-btn {
		position: absolute;
		top: 50%;
		transform: translateY(-50%);
		width: 3rem;
		height: 3rem;
		border: none;
		background: rgba(255, 255, 255, 0.1);
		border-radius: 50%;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: background 0.2s;
		z-index: 10;
	}

	.nav-btn svg {
		width: 1.5rem;
		height: 1.5rem;
		color: white;
	}

	.nav-btn:hover {
		background: rgba(255, 255, 255, 0.2);
	}

	.nav-prev {
		left: 1rem;
	}

	.nav-next {
		right: 1rem;
	}

	.lightbox-content {
		display: flex;
		gap: 2rem;
		max-width: 90vw;
		max-height: 80vh;
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
		max-height: 80vh;
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
		font-size: 1.125rem;
		font-weight: 600;
	}

	.description {
		margin: 0 0 1.5rem;
		color: #cbd5e1;
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
		color: #94a3b8;
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

	.media-counter {
		margin-top: 1.5rem;
		padding-top: 1rem;
		border-top: 1px solid rgba(255, 255, 255, 0.1);
		text-align: center;
		color: #94a3b8;
		font-size: 0.875rem;
	}

	.btn {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.625rem 1rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
		font-weight: 500;
		color: #475569;
		cursor: pointer;
		text-decoration: none;
		transition: all 0.15s;
	}

	.btn svg {
		width: 1rem;
		height: 1rem;
	}

	.btn-primary {
		background: #3b82f6;
		border-color: #3b82f6;
		color: white;
	}

	.btn-primary:hover {
		background: #2563eb;
	}

	@media (max-width: 768px) {
		.lightbox-overlay {
			padding: 1rem;
		}

		.lightbox-content {
			flex-direction: column;
			gap: 1rem;
		}

		.metadata-panel {
			width: 100%;
			max-height: 200px;
		}

		.nav-btn {
			width: 2.5rem;
			height: 2.5rem;
		}

		.nav-prev {
			left: 0.5rem;
		}

		.nav-next {
			right: 0.5rem;
		}
	}
</style>
