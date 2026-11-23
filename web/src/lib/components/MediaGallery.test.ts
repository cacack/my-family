import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/svelte';
import MediaGallery from './MediaGallery.svelte';
import * as apiModule from '#lib/api/client.js';
import type { Media } from '#lib/api/client.js';
import { auth } from '#lib/stores/access.svelte.js';

const branchState = vi.hoisted(() => ({ id: null as string | null }));
vi.mock('#lib/stores/activeBranch.svelte.js', () => ({ activeBranch: branchState }));

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			listPersonMedia: vi.fn(),
			listFamilyMedia: vi.fn(),
			listSourceMedia: vi.fn(),
			uploadPersonMedia: vi.fn(),
			uploadFamilyMedia: vi.fn(),
			uploadSourceMedia: vi.fn(),
			updateMedia: vi.fn(),
			attachMediaContent: vi.fn(),
			getMedia: vi.fn(),
			getMediaThumbnailUrl: (id: string) => `/api/v1/media/${id}/thumbnail`,
			getMediaContentUrl: (id: string) => `/api/v1/media/${id}/content`
		}
	};
});

function media(overrides: Partial<Media>): Media {
	return {
		id: 'm-1',
		entity_type: 'person',
		entity_id: 'p-1',
		title: 'Portrait',
		mime_type: 'image/jpeg',
		filename: 'portrait.jpg',
		file_size: 2048,
		has_content: true,
		has_thumbnail: true,
		version: 1,
		created_at: '2026-01-01T00:00:00Z',
		...overrides
	};
}

// A GEDCOM-imported item names its file but stores no content (#1477).
const reference = media({
	id: 'm-ref',
	title: 'Wedding',
	filename: '',
	file_size: 0,
	has_content: false,
	has_thumbnail: false,
	files: [{ path: 'photos/wedding.jpg', format: 'image/jpeg' }]
});

function list(items: Media[]) {
	vi.mocked(apiModule.api.listPersonMedia).mockResolvedValue({ items, total: items.length });
}

describe('MediaGallery', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
	});

	it('shows a reference-only item as a placeholder with its title and path, not an image', async () => {
		list([reference]);
		render(MediaGallery, { props: { ownerType: 'person', ownerId: 'p-1' } });

		const card = await screen.findByRole('button', { name: /^View Wedding photos\/wedding\.jpg File not stored$/ });
		expect(within(card).getByText('Wedding')).toBeDefined();
		expect(within(card).getByText('photos/wedding.jpg')).toBeDefined();
		expect(within(card).getByText('File not stored')).toBeDefined();
		expect(card.querySelector('img')).toBeNull();
	});

	it('shows an uploaded item with a thumbnail as its image', async () => {
		list([media({})]);
		render(MediaGallery, { props: { ownerType: 'person', ownerId: 'p-1' } });

		const card = await screen.findByRole('button', { name: 'View Portrait' });
		expect(card.querySelector('img')?.getAttribute('src')).toBe('/api/v1/media/m-1/thumbnail');
		expect(within(card).queryByText('File not stored')).toBeNull();
	});

	it('opens a reference-only item without loading or offering its content', async () => {
		list([reference]);
		render(MediaGallery, { props: { ownerType: 'person', ownerId: 'p-1' } });

		await fireEvent.click(await screen.findByRole('button', { name: /^View Wedding photos\/wedding\.jpg File not stored$/ }));

		const dialog = await screen.findByRole('dialog');
		expect(within(dialog).getByText('photos/wedding.jpg')).toBeDefined();
		expect(within(dialog).getByText('Not stored (reference only)')).toBeDefined();
		expect(dialog.querySelector('img')).toBeNull();
		expect(dialog.querySelector('a[href*="/content"]')).toBeNull();
	});

	it('shows a reference whose FILE lines repeat a path or omit it', async () => {
		list([
			media({
				...reference,
				files: [
					{ path: 'photos/wedding.jpg', format: 'image/jpeg' },
					{ path: 'photos/wedding.jpg', format: 'image/png' },
					{ format: 'image/jpeg' },
					{ format: 'image/png' }
				]
			})
		]);
		render(MediaGallery, { props: { ownerType: 'person', ownerId: 'p-1' } });

		const card = await screen.findByRole('button', { name: /^View Wedding/ });
		expect(within(card).getAllByText('photos/wedding.jpg')).toHaveLength(2);

		await fireEvent.click(card);
		const dialog = await screen.findByRole('dialog');
		expect(within(dialog).getAllByText('photos/wedding.jpg')).toHaveLength(2);
	});
	describe('the private mark (#1413)', () => {
		async function openPortrait(item: Media) {
			list([item]);
			render(MediaGallery, { props: { ownerType: 'person', ownerId: 'p-1' } });
			await fireEvent.click(await screen.findByRole('button', { name: 'View Portrait' }));
			return screen.findByRole('dialog');
		}

		it('marks a private item in the gallery with a named badge', async () => {
			list([media({ private: true }), media({ id: 'm-2', title: 'Letter' })]);
			render(MediaGallery, { props: { ownerType: 'person', ownerId: 'p-1' } });

			expect(await screen.findAllByRole('img', { name: 'Private media item' })).toHaveLength(1);
		});

		it('marks an item private from its lightbox and shows the badge', async () => {
			vi.mocked(apiModule.api.updateMedia).mockResolvedValue(media({ private: true, version: 2 }));
			const dialog = await openPortrait(media({}));
			expect(within(dialog).getByText('Not private')).toBeDefined();

			await fireEvent.click(within(dialog).getByRole('button', { name: 'Mark private' }));

			expect(apiModule.api.updateMedia).toHaveBeenCalledWith('m-1', { private: true, version: 1 });
			expect(await within(dialog).findByRole('button', { name: 'Unmark private' })).toBeDefined();
			expect(within(dialog).getByText(/^Private: viewers and visitors do not see it$/)).toBeDefined();
			expect(screen.getByRole('img', { name: 'Private media item' })).toBeDefined();
		});

		it('unmarks a private item with its current version', async () => {
			vi.mocked(apiModule.api.updateMedia).mockResolvedValue(media({ private: false, version: 4 }));
			const dialog = await openPortrait(media({ private: true, version: 3 }));

			await fireEvent.click(within(dialog).getByRole('button', { name: 'Unmark private' }));

			expect(apiModule.api.updateMedia).toHaveBeenCalledWith('m-1', { private: false, version: 3 });
			expect(await within(dialog).findByRole('button', { name: 'Mark private' })).toBeDefined();
			expect(screen.queryByRole('img', { name: 'Private media item' })).toBeNull();
		});

		it('reloads a stale item and resends the same mark on Try again', async () => {
			vi.mocked(apiModule.api.updateMedia).mockRejectedValueOnce({
				status: 409,
				code: 'VERSION_CONFLICT',
				message: 'version conflict'
			});
			vi.mocked(apiModule.api.getMedia).mockResolvedValue(media({ version: 5 }));
			vi.mocked(apiModule.api.updateMedia).mockResolvedValueOnce(media({ private: true, version: 6 }));
			const dialog = await openPortrait(media({}));

			await fireEvent.click(within(dialog).getByRole('button', { name: 'Mark private' }));
			expect((await within(dialog).findByRole('alert')).textContent).toMatch(/changed elsewhere/);

			await fireEvent.click(within(dialog).getByRole('button', { name: 'Try again: mark private' }));
			expect(apiModule.api.updateMedia).toHaveBeenLastCalledWith('m-1', { private: true, version: 5 });
			expect(await within(dialog).findByRole('button', { name: 'Unmark private' })).toBeDefined();
			expect(within(dialog).queryByRole('button', { name: /^Try again/ })).toBeNull();
		});

		it('resends the mark when a grouped copy failed after this item was saved', async () => {
			// The item itself was saved private before a grouped copy conflicted,
			// so the reload shows it private; Try again must still send the mark.
			vi.mocked(apiModule.api.updateMedia).mockRejectedValueOnce({
				status: 409,
				code: 'VERSION_CONFLICT',
				message: 'version conflict'
			});
			vi.mocked(apiModule.api.getMedia).mockResolvedValue(media({ private: true, version: 2 }));
			vi.mocked(apiModule.api.updateMedia).mockResolvedValueOnce(media({ private: true, version: 2 }));
			const dialog = await openPortrait(media({}));

			await fireEvent.click(within(dialog).getByRole('button', { name: 'Mark private' }));
			await fireEvent.click(await within(dialog).findByRole('button', { name: 'Try again: mark private' }));

			expect(apiModule.api.updateMedia).toHaveBeenLastCalledWith('m-1', { private: true, version: 2 });
		});

		it('shows a refusal in the lightbox', async () => {
			vi.mocked(apiModule.api.updateMedia).mockRejectedValue({ status: 403, message: 'Not allowed' });
			const dialog = await openPortrait(media({}));

			await fireEvent.click(within(dialog).getByRole('button', { name: 'Mark private' }));

			expect((await within(dialog).findByRole('alert')).textContent).toBe('Not allowed');
		});
	});
	describe('editing the details (#1107)', () => {
		async function open(item: Media, name: string | RegExp) {
			list([item]);
			render(MediaGallery, { props: { ownerType: 'person', ownerId: 'p-1' } });
			await fireEvent.click(await screen.findByRole('button', { name }));
			return screen.findByRole('dialog');
		}

		async function fill(dialog: HTMLElement, title: string, description: string, type: string) {
			await fireEvent.click(within(dialog).getByRole('button', { name: 'Edit details' }));
			await fireEvent.input(within(dialog).getByRole('textbox', { name: /^Title/ }), { target: { value: title } });
			await fireEvent.input(within(dialog).getByRole('textbox', { name: 'Description' }), {
				target: { value: description }
			});
			await fireEvent.change(within(dialog).getByRole('combobox', { name: 'Type' }), { target: { value: type } });
		}

		it('saves a new title, description and type with the item version', async () => {
			vi.mocked(apiModule.api.updateMedia).mockResolvedValue(
				media({ title: 'Grandma, 1920', description: 'On the porch', media_type: 'document', version: 3 })
			);
			const dialog = await open(media({ media_type: 'photo', version: 2 }), 'View Portrait');

			await fill(dialog, 'Grandma, 1920', 'On the porch', 'document');
			await fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));

			expect(apiModule.api.updateMedia).toHaveBeenCalledWith('m-1', {
				title: 'Grandma, 1920',
				description: 'On the porch',
				media_type: 'document',
				version: 2
			});
			expect(await within(dialog).findByRole('heading', { name: 'Grandma, 1920' })).toBeDefined();
			expect(within(dialog).getByText('On the porch')).toBeDefined();
			expect(within(dialog).getByText('document')).toBeDefined();
			expect(within(dialog).queryByRole('form', { name: 'Edit details' })).toBeNull();
			expect(screen.getByRole('button', { name: 'View Grandma, 1920' })).toBeDefined();
		});

		it('edits a GEDCOM-imported reference, which has no file', async () => {
			vi.mocked(apiModule.api.updateMedia).mockResolvedValue(
				media({ ...reference, title: 'Wedding, 1890', media_type: 'certificate', version: 2 })
			);
			const dialog = await open(reference, /^View Wedding/);

			await fill(dialog, 'Wedding, 1890', '', 'certificate');
			await fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));

			expect(apiModule.api.updateMedia).toHaveBeenCalledWith('m-ref', {
				title: 'Wedding, 1890',
				description: null,
				media_type: 'certificate',
				version: 1
			});
			expect(await within(dialog).findByRole('heading', { name: 'Wedding, 1890' })).toBeDefined();
			expect(within(dialog).getByText('Not stored (reference only)')).toBeDefined();
			expect(screen.getByRole('button', { name: /^View Wedding, 1890 photos\/wedding\.jpg/ })).toBeDefined();
		});

		it('keeps what was typed after a stale save and resends it on Try again', async () => {
			vi.mocked(apiModule.api.updateMedia).mockRejectedValueOnce({
				status: 409,
				code: 'VERSION_CONFLICT',
				message: 'version conflict'
			});
			vi.mocked(apiModule.api.getMedia).mockResolvedValue(media({ title: 'Portrait (edited elsewhere)', version: 5 }));
			vi.mocked(apiModule.api.updateMedia).mockResolvedValueOnce(media({ title: 'Grandma', version: 6 }));
			const dialog = await open(media({}), 'View Portrait');

			await fill(dialog, 'Grandma', 'On the porch', 'photo');
			await fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));

			expect((await within(dialog).findByRole('alert')).textContent).toMatch(/changed elsewhere/);
			expect((within(dialog).getByRole('textbox', { name: /^Title/ }) as HTMLInputElement).value).toBe('Grandma');
			expect((within(dialog).getByRole('textbox', { name: 'Description' }) as HTMLTextAreaElement).value).toBe(
				'On the porch'
			);

			await fireEvent.click(within(dialog).getByRole('button', { name: 'Try again' }));
			expect(apiModule.api.updateMedia).toHaveBeenLastCalledWith('m-1', {
				title: 'Grandma',
				description: 'On the porch',
				media_type: 'photo',
				version: 5
			});
			expect(await within(dialog).findByRole('heading', { name: 'Grandma' })).toBeDefined();
			expect(within(dialog).queryByRole('button', { name: 'Try again' })).toBeNull();
		});

		it('shows a refusal and keeps the form', async () => {
			vi.mocked(apiModule.api.updateMedia).mockRejectedValue({ status: 400, message: 'title too long' });
			const dialog = await open(media({}), 'View Portrait');

			await fill(dialog, 'Grandma', '', 'photo');
			await fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));

			expect((await within(dialog).findByRole('alert')).textContent).toBe('title too long');
			expect((within(dialog).getByRole('textbox', { name: /^Title/ }) as HTMLInputElement).value).toBe('Grandma');
		});

		it('leaves the item unchanged on Cancel', async () => {
			const dialog = await open(media({}), 'View Portrait');

			await fill(dialog, 'Grandma', '', 'photo');
			await fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));

			expect(apiModule.api.updateMedia).not.toHaveBeenCalled();
			expect(within(dialog).getByRole('heading', { name: 'Portrait' })).toBeDefined();
			expect(within(dialog).getByRole('button', { name: 'Edit details' })).toBeDefined();
		});

		it('does not save an empty title', async () => {
			const dialog = await open(media({}), 'View Portrait');

			await fill(dialog, '   ', '', 'photo');

			expect((within(dialog).getByRole('button', { name: 'Save' }) as HTMLButtonElement).disabled).toBe(true);
		});

		it('is not offered to a viewer', async () => {
			auth.phase = 'visitor';
			const dialog = await open(media({}), 'View Portrait');

			expect(within(dialog).queryByRole('button', { name: 'Edit details' })).toBeNull();
			expect(within(dialog).queryByRole('textbox')).toBeNull();
		});
	});
	describe('attaching a file to a reference (#1504)', () => {
		async function openReference() {
			list([reference]);
			render(MediaGallery, { props: { ownerType: 'person', ownerId: 'p-1' } });
			await fireEvent.click(await screen.findByRole('button', { name: /^View Wedding/ }));
			return screen.findByRole('dialog');
		}

		function chooseFile(dialog: HTMLElement, file: File) {
			const input = dialog.querySelector('input[type="file"]') as HTMLInputElement;
			Object.defineProperty(input, 'files', { value: [file], configurable: true });
			return fireEvent.change(input);
		}

		it('attaches a file and then shows the item as its image, not a reference', async () => {
			const attached = media({
				...reference,
				filename: 'wedding.jpg',
				file_size: 2048,
				has_content: true,
				has_thumbnail: true,
				version: 2
			});
			vi.mocked(apiModule.api.attachMediaContent).mockResolvedValue(attached);
			const dialog = await openReference();
			expect(within(dialog).getByRole('button', { name: 'Attach file' })).toBeDefined();

			const file = new File(['jpeg'], 'wedding.jpg', { type: 'image/jpeg' });
			await chooseFile(dialog, file);

			expect(apiModule.api.attachMediaContent).toHaveBeenCalledWith('m-ref', file);
			const img = await within(dialog).findByRole('img', { name: 'Wedding' });
			expect(img.getAttribute('src')).toBe('/api/v1/media/m-ref/content');
			expect(within(dialog).queryByText('Not stored (reference only)')).toBeNull();
			expect(within(dialog).queryByRole('button', { name: 'Attach file' })).toBeNull();
			const card = screen.getByRole('button', { name: 'View Wedding' });
			expect(card.querySelector('img')?.getAttribute('src')).toBe('/api/v1/media/m-ref/thumbnail');
		});

		it('shows a refused file and keeps the reference', async () => {
			vi.mocked(apiModule.api.attachMediaContent).mockRejectedValue({
				status: 400,
				message: 'unsupported file type'
			});
			const dialog = await openReference();

			await chooseFile(dialog, new File(['text'], 'notes.txt', { type: 'text/plain' }));

			expect((await within(dialog).findByRole('alert')).textContent).toBe('unsupported file type');
			expect(within(dialog).getByText('Not stored (reference only)')).toBeDefined();
		});

		it('is not offered to a viewer', async () => {
			auth.phase = 'visitor';
			const dialog = await openReference();

			expect(within(dialog).queryByRole('button', { name: 'Attach file' })).toBeNull();
			expect(dialog.querySelector('input[type="file"]')).toBeNull();
		});

		it('is not offered on a research branch, as attaching is mainline-only', async () => {
			branchState.id = 'b-1';
			const dialog = await openReference();

			expect(within(dialog).queryByRole('button', { name: 'Attach file' })).toBeNull();
		});

		it('is not offered for an item that has its file', async () => {
			list([media({})]);
			render(MediaGallery, { props: { ownerType: 'person', ownerId: 'p-1' } });
			await fireEvent.click(await screen.findByRole('button', { name: 'View Portrait' }));
			const dialog = await screen.findByRole('dialog');

			expect(within(dialog).queryByRole('button', { name: 'Attach file' })).toBeNull();
		});
	});

	describe('the owner (#1505, #1538)', () => {
		it.each([
			['family', 'f-1', 'listFamilyMedia'],
			['source', 's-1', 'listSourceMedia']
		] as const)("lists a %s's media and shows its reference", async (ownerType, ownerId, lister) => {
			vi.mocked(apiModule.api[lister]).mockResolvedValue({ items: [reference], total: 1 });
			render(MediaGallery, { props: { ownerType, ownerId } });

			const card = await screen.findByRole('button', { name: /^View Wedding photos\/wedding\.jpg File not stored$/ });
			expect(card).toBeDefined();
			expect(apiModule.api[lister]).toHaveBeenCalledWith(ownerId);
			expect(apiModule.api.listPersonMedia).not.toHaveBeenCalled();
		});

		it.each([
			['person', 'p-1', 'listPersonMedia', 'uploadPersonMedia'],
			['family', 'f-1', 'listFamilyMedia', 'uploadFamilyMedia'],
			['source', 's-1', 'listSourceMedia', 'uploadSourceMedia']
		] as const)('uploads to a %s as an editor and shows the new item', async (ownerType, ownerId, lister, uploader) => {
			vi.mocked(apiModule.api[lister]).mockResolvedValue({ items: [], total: 0 });
			const uploaded = media({ id: 'm-new', entity_type: ownerType, entity_id: ownerId, title: 'certificate' });
			vi.mocked(apiModule.api[uploader]).mockResolvedValue(uploaded);
			render(MediaGallery, { props: { ownerType, ownerId } });

			expect((await screen.findByText(/No media yet/)).textContent).toBe('No media yet. Upload photos and documents.');
			const input = document.querySelector('input[type="file"]') as HTMLInputElement;
			const file = new File(['x'], 'certificate.jpg', { type: 'image/jpeg' });
			Object.defineProperty(input, 'files', { value: [file], configurable: true });
			await fireEvent.change(input);
			await fireEvent.click(screen.getByRole('button', { name: 'Upload' }));

			expect(apiModule.api[uploader]).toHaveBeenCalledWith(ownerId, file, 'certificate', undefined, 'photo');
			expect(await screen.findByRole('button', { name: 'View certificate' })).toBeDefined();
		});

		it.each(['person', 'family', 'source'] as const)('offers a viewer no upload on a %s', async (ownerType) => {
			auth.phase = 'visitor';
			for (const lister of ['listPersonMedia', 'listFamilyMedia', 'listSourceMedia'] as const) {
				vi.mocked(apiModule.api[lister]).mockResolvedValue({ items: [], total: 0 });
			}
			render(MediaGallery, { props: { ownerType, ownerId: 'x-1' } });

			expect((await screen.findByText(/No media yet/)).textContent).toBe('No media yet.');
			expect(screen.queryByRole('heading', { name: 'Upload Media' })).toBeNull();
			expect(document.querySelector('input[type="file"]')).toBeNull();
		});
	});
});
