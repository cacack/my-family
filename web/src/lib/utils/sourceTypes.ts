/**
 * Every source type the API accepts (domain.SourceType), in the order the
 * source forms offer them. The API rejects any other value.
 */
export const SOURCE_TYPES = [
	{ value: 'book', label: 'Book' },
	{ value: 'archive', label: 'Archive' },
	{ value: 'webpage', label: 'Webpage' },
	{ value: 'census', label: 'Census' },
	{ value: 'vital_record', label: 'Vital Record' },
	{ value: 'church_record', label: 'Church Record' },
	{ value: 'newspaper', label: 'Newspaper' },
	{ value: 'photograph', label: 'Photograph' },
	{ value: 'interview', label: 'Interview' },
	{ value: 'correspondence', label: 'Correspondence' },
	{ value: 'other', label: 'Other' }
] as const;

/** The source type a new source starts with. */
export const DEFAULT_SOURCE_TYPE = 'other';
