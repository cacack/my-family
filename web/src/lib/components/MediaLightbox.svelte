<script lang="ts">
	import { api, type Media } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';

	interface Props {
		open: boolean;
		media: Media;
		allMedia: Media[];
		onClose: () => void;
	}

	let { open = $bindable(), media, allMedia, onClose }: Props = $props();

	let currentIndex = $derived(allMedia.findIndex((m) => m.id === media.id));
	let currentMedia = $derived(allMedia[currentIndex] || media);
	let isImage = $derived(currentMedia.mime_type.startsWith('image/'));
	let isPdf = $derived(currentMedia.mime_type === 'application/pdf');

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
	<Dialog.Content
		showCloseButton={false}
		class="max-w-[95vw] h-[90vh] p-0 sm:max-w-[95vw] bg-black/95 border-none"
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

	@media (max-width: 768px) {
		.lightbox-content {
			flex-direction: column;
			gap: 1rem;
			padding: 1rem;
		}

		.metadata-panel {
			width: 100%;
			max-height: 200px;
		}
	}
</style>
