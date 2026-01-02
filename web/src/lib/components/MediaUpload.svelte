<script lang="ts">
	import { api, type Media } from '$lib/api/client';

	interface Props {
		personId: string;
		onUpload?: (media: Media) => void;
	}

	let { personId, onUpload }: Props = $props();

	let file: File | null = $state(null);
	let title = $state('');
	let description = $state('');
	let mediaType = $state('photo');
	let dragOver = $state(false);
	let uploading = $state(false);
	let error: string | null = $state(null);
	let success = $state(false);

	const MAX_FILE_SIZE = 10 * 1024 * 1024; // 10MB
	const ALLOWED_IMAGE_TYPES = ['image/jpeg', 'image/png', 'image/gif', 'image/webp'];
	const ALLOWED_DOCUMENT_TYPES = [
		'application/pdf',
		'application/msword',
		'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
		'text/plain'
	];
	const ALLOWED_TYPES = [...ALLOWED_IMAGE_TYPES, ...ALLOWED_DOCUMENT_TYPES];

	function validateFile(f: File): string | null {
		if (f.size > MAX_FILE_SIZE) {
			return `File is too large. Maximum size is 10MB. Your file is ${(f.size / 1024 / 1024).toFixed(1)}MB.`;
		}
		if (!ALLOWED_TYPES.includes(f.type)) {
			return 'File type not supported. Please upload an image (JPEG, PNG, GIF, WebP) or document (PDF, Word, TXT).';
		}
		return null;
	}

	function handleFileSelect(e: Event) {
		const input = e.target as HTMLInputElement;
		if (input.files && input.files.length > 0) {
			selectFile(input.files[0]);
		}
	}

	function selectFile(f: File) {
		error = null;
		success = false;
		const validationError = validateFile(f);
		if (validationError) {
			error = validationError;
			file = null;
			return;
		}
		file = f;
		// Auto-set title to filename without extension if empty
		if (!title) {
			title = f.name.replace(/\.[^/.]+$/, '');
		}
		// Auto-detect media type based on file type
		if (ALLOWED_IMAGE_TYPES.includes(f.type)) {
			mediaType = 'photo';
		} else {
			mediaType = 'document';
		}
	}

	function handleDrop(e: DragEvent) {
		e.preventDefault();
		dragOver = false;
		if (e.dataTransfer?.files && e.dataTransfer.files.length > 0) {
			selectFile(e.dataTransfer.files[0]);
		}
	}

	function handleDragOver(e: DragEvent) {
		e.preventDefault();
		dragOver = true;
	}

	function handleDragLeave() {
		dragOver = false;
	}

	function removeFile() {
		file = null;
		title = '';
		description = '';
		mediaType = 'photo';
		error = null;
		success = false;
	}

	async function uploadFile() {
		if (!file || !title.trim()) return;

		uploading = true;
		error = null;
		success = false;

		try {
			const media = await api.uploadPersonMedia(
				personId,
				file,
				title.trim(),
				description.trim() || undefined,
				mediaType
			);
			success = true;
			onUpload?.(media);
			// Reset form for next upload
			file = null;
			title = '';
			description = '';
			mediaType = 'photo';
		} catch (e) {
			error = (e as { message?: string }).message || 'Upload failed. Please try again.';
		} finally {
			uploading = false;
		}
	}

	function formatFileSize(bytes: number): string {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
	}
</script>

<div class="media-upload">
	<h3>Upload Media</h3>

	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="dropzone"
		class:dragover={dragOver}
		class:has-file={file !== null}
		ondrop={handleDrop}
		ondragover={handleDragOver}
		ondragleave={handleDragLeave}
	>
		{#if file}
			<div class="file-info">
				<svg class="file-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					{#if ALLOWED_IMAGE_TYPES.includes(file.type)}
						<rect x="3" y="3" width="18" height="18" rx="2" ry="2" />
						<circle cx="8.5" cy="8.5" r="1.5" />
						<polyline points="21 15 16 10 5 21" />
					{:else}
						<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
						<polyline points="14 2 14 8 20 8" />
					{/if}
				</svg>
				<div class="file-details">
					<span class="file-name">{file.name}</span>
					<span class="file-size">{formatFileSize(file.size)}</span>
				</div>
				<button class="remove-btn" onclick={removeFile} type="button" aria-label="Remove file">
					<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<line x1="18" y1="6" x2="6" y2="18" />
						<line x1="6" y1="6" x2="18" y2="18" />
					</svg>
				</button>
			</div>
		{:else}
			<svg class="upload-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
				<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
				<polyline points="17 8 12 3 7 8" />
				<line x1="12" y1="3" x2="12" y2="15" />
			</svg>
			<p class="dropzone-text">Drag and drop a file here, or</p>
			<label class="file-label">
				<input
					type="file"
					accept="image/jpeg,image/png,image/gif,image/webp,.pdf,.doc,.docx,.txt"
					onchange={handleFileSelect}
					hidden
				/>
				Browse Files
			</label>
			<p class="dropzone-hint">Images, PDFs, and documents up to 10MB</p>
		{/if}
	</div>

	{#if file}
		<div class="upload-form">
			<label class="form-field">
				<span class="field-label">Title <span class="required">*</span></span>
				<input type="text" bind:value={title} placeholder="Enter a title for this media" required />
			</label>

			<label class="form-field">
				<span class="field-label">Description</span>
				<textarea bind:value={description} placeholder="Optional description" rows="2"></textarea>
			</label>

			<label class="form-field">
				<span class="field-label">Type</span>
				<select bind:value={mediaType}>
					<option value="photo">Photo</option>
					<option value="document">Document</option>
					<option value="certificate">Certificate</option>
				</select>
			</label>

			<button
				class="btn btn-primary btn-upload"
				onclick={uploadFile}
				disabled={uploading || !title.trim()}
			>
				{#if uploading}
					<span class="spinner"></span>
					Uploading...
				{:else}
					Upload
				{/if}
			</button>
		</div>
	{/if}

	{#if error}
		<div class="message error-message">{error}</div>
	{/if}

	{#if success}
		<div class="message success-message">Media uploaded successfully!</div>
	{/if}
</div>

<style>
	.media-upload {
		background: #f8fafc;
		border-radius: 8px;
		padding: 1rem;
		margin-bottom: 1.5rem;
	}

	h3 {
		margin: 0 0 1rem;
		font-size: 0.9375rem;
		font-weight: 600;
		color: #475569;
	}

	.dropzone {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		padding: 2rem;
		border: 2px dashed #cbd5e1;
		border-radius: 8px;
		background: white;
		transition: all 0.2s;
		cursor: pointer;
	}

	.dropzone.dragover {
		border-color: #3b82f6;
		background: #eff6ff;
	}

	.dropzone.has-file {
		padding: 1rem;
		cursor: default;
	}

	.upload-icon {
		width: 2.5rem;
		height: 2.5rem;
		color: #94a3b8;
		margin-bottom: 0.75rem;
	}

	.dropzone-text {
		margin: 0 0 0.5rem;
		color: #64748b;
		font-size: 0.875rem;
	}

	.dropzone-hint {
		margin: 0.5rem 0 0;
		color: #94a3b8;
		font-size: 0.75rem;
	}

	.file-label {
		display: inline-block;
		padding: 0.5rem 1rem;
		background: white;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		font-size: 0.875rem;
		font-weight: 500;
		color: #475569;
		cursor: pointer;
		transition: all 0.15s;
	}

	.file-label:hover {
		background: #f1f5f9;
		border-color: #94a3b8;
	}

	.file-info {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		width: 100%;
	}

	.file-icon {
		width: 2rem;
		height: 2rem;
		color: #3b82f6;
		flex-shrink: 0;
	}

	.file-details {
		flex: 1;
		min-width: 0;
	}

	.file-name {
		display: block;
		font-weight: 500;
		color: #1e293b;
		font-size: 0.875rem;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.file-size {
		color: #64748b;
		font-size: 0.75rem;
	}

	.remove-btn {
		width: 1.75rem;
		height: 1.75rem;
		border: none;
		background: #f1f5f9;
		border-radius: 50%;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		transition: all 0.15s;
	}

	.remove-btn svg {
		width: 1rem;
		height: 1rem;
		color: #64748b;
	}

	.remove-btn:hover {
		background: #fee2e2;
	}

	.remove-btn:hover svg {
		color: #dc2626;
	}

	.upload-form {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		margin-top: 1rem;
	}

	.form-field {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.field-label {
		font-size: 0.8125rem;
		font-weight: 500;
		color: #475569;
	}

	.required {
		color: #dc2626;
	}

	input[type="text"],
	textarea,
	select {
		padding: 0.5rem 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		font-size: 0.875rem;
		background: white;
	}

	input[type="text"]:focus,
	textarea:focus,
	select:focus {
		outline: none;
		border-color: #3b82f6;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
	}

	textarea {
		resize: vertical;
	}

	.btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 0.5rem;
		padding: 0.625rem 1rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
		font-weight: 500;
		color: #475569;
		cursor: pointer;
		transition: all 0.15s;
	}

	.btn:hover {
		background: #f1f5f9;
	}

	.btn-primary {
		background: #3b82f6;
		border-color: #3b82f6;
		color: white;
	}

	.btn-primary:hover {
		background: #2563eb;
	}

	.btn-primary:disabled {
		background: #93c5fd;
		border-color: #93c5fd;
		cursor: not-allowed;
	}

	.btn-upload {
		margin-top: 0.5rem;
	}

	.spinner {
		width: 1rem;
		height: 1rem;
		border: 2px solid rgba(255, 255, 255, 0.3);
		border-top-color: white;
		border-radius: 50%;
		animation: spin 0.6s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	.message {
		margin-top: 0.75rem;
		padding: 0.625rem 0.875rem;
		border-radius: 6px;
		font-size: 0.875rem;
	}

	.error-message {
		background: #fef2f2;
		color: #dc2626;
		border: 1px solid #fecaca;
	}

	.success-message {
		background: #f0fdf4;
		color: #166534;
		border: 1px solid #86efac;
	}
</style>
