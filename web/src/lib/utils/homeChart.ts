import { redirect } from '@sveltejs/kit';
import { api } from '#lib/api/client.js';

/** The charts that can start from the home person (#1034). */
export type HomeChart = 'pedigree' | 'descendancy' | 'ahnentafel';

/**
 * The load for a chart route without a person: it starts from the home person
 * when one is set. Otherwise (or when the setting cannot be read) the route
 * renders and explains how to pick one.
 */
export async function loadHomeChart(chart: HomeChart): Promise<Record<string, never>> {
	let personId: string | undefined;
	try {
		personId = (await api.getHomePerson()).person?.id;
	} catch {
		return {};
	}
	if (personId) redirect(307, `/${chart}/${encodeURIComponent(personId)}`);
	return {};
}
