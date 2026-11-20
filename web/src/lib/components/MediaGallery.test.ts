import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/svelte';
import MediaGallery from './MediaGallery.svelte';
import * as apiModule from '#lib/api/client.js';
import type { Media } from '#lib/api/client.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			listPersonMedia: vi.fn(),
			updateMedia: vi.fn(),
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
	has_thumbnail: false,
	files: [{ path: 'photos/wedding.jpg', format: 'image/jpeg' }]
});

function list(items: Media[]) {
	vi.mocked(apiModule.api.listPersonMedia).mockResolvedValue({ items, total: items.length });
}

describe('MediaGallery', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('shows a reference-only item as a placeholder with its title and path, not an image', async () => {
		list([reference]);
		render(MediaGallery, { props: { personId: 'p-1' } });

		const card = await screen.findByRole('button', { name: /^View Wedding photos\/wedding\.jpg File not stored$/ });
		expect(within(card).getByText('Wedding')).toBeDefined();
		expect(within(card).getByText('photos/wedding.jpg')).toBeDefined();
		expect(within(card).getByText('File not stored')).toBeDefined();
		expect(card.querySelector('img')).toBeNull();
	});

	it('shows an uploaded item with a thumbnail as its image', async () => {
		list([media({})]);
		render(MediaGallery, { props: { personId: 'p-1' } });

		const card = await screen.findByRole('button', { name: 'View Portrait' });
		expect(card.querySelector('img')?.getAttribute('src')).toBe('/api/v1/media/m-1/thumbnail');
		expect(within(card).queryByText('File not stored')).toBeNull();
	});

	it('opens a reference-only item without loading or offering its content', async () => {
		list([reference]);
		render(MediaGallery, { props: { personId: 'p-1' } });

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
		render(MediaGallery, { props: { personId: 'p-1' } });

		const card = await screen.findByRole('button', { name: /^View Wedding/ });
		expect(within(card).getAllByText('photos/wedding.jpg')).toHaveLength(2);

		await fireEvent.click(card);
		const dialog = await screen.findByRole('dialog');
		expect(within(dialog).getAllByText('photos/wedding.jpg')).toHaveLength(2);
	});
	describe('the private mark (#1413)', () => {
		async function openPortrait(item: Media) {
			list([item]);
			render(MediaGallery, { props: { personId: 'p-1' } });
			await fireEvent.click(await screen.findByRole('button', { name: 'View Portrait' }));
			return screen.findByRole('dialog');
		}

		it('marks a private item in the gallery with a named badge', async () => {
			list([media({ private: true }), media({ id: 'm-2', title: 'Letter' })]);
			render(MediaGallery, { props: { personId: 'p-1' } });

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

		it('reloads a stale item and asks the editor to try again', async () => {
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

			await fireEvent.click(within(dialog).getByRole('button', { name: 'Mark private' }));
			expect(apiModule.api.updateMedia).toHaveBeenLastCalledWith('m-1', { private: true, version: 5 });
			expect(await within(dialog).findByRole('button', { name: 'Unmark private' })).toBeDefined();
		});

		it('shows a refusal in the lightbox', async () => {
			vi.mocked(apiModule.api.updateMedia).mockRejectedValue({ status: 403, message: 'Not allowed' });
			const dialog = await openPortrait(media({}));

			await fireEvent.click(within(dialog).getByRole('button', { name: 'Mark private' }));

			expect((await within(dialog).findByRole('alert')).textContent).toBe('Not allowed');
		});
	});
});
