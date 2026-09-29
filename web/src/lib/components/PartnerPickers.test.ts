import { describe, it, expect } from 'vitest';
import { partnerChanges } from './PartnerPickers.svelte';

const ann = { id: 'ann-id', given_name: 'Ann', surname: 'Smith' };
const john = { id: 'john-id', given_name: 'John', surname: 'Smith' };

describe('partnerChanges', () => {
	it('sends nothing when the partners are unchanged', () => {
		expect(partnerChanges({ partner1_id: 'john-id', partner2_id: 'ann-id' }, john, ann)).toEqual({});
		expect(partnerChanges({}, null, null)).toEqual({});
	});

	it('sends the id of a newly picked partner', () => {
		expect(partnerChanges({}, john, null)).toEqual({ partner1_id: 'john-id' });
		expect(partnerChanges({ partner1_id: 'john-id' }, john, ann)).toEqual({ partner2_id: 'ann-id' });
	});

	it('clears a removed partner rather than omitting it', () => {
		expect(partnerChanges({ partner1_id: 'john-id', partner2_id: 'ann-id' }, john, null)).toEqual({
			clear_partner2: true
		});
		expect(partnerChanges({ partner1_id: 'john-id' }, null, null)).toEqual({ clear_partner1: true });
	});

	it('sends both ids for a swap', () => {
		expect(partnerChanges({ partner1_id: 'john-id', partner2_id: 'ann-id' }, ann, john)).toEqual({
			partner1_id: 'ann-id',
			partner2_id: 'john-id'
		});
	});
});
