/**
 * Presentation helpers for relationship calculator results.
 */
import type { RelationshipPath, RelationshipPathNode, RelationshipResult } from '#lib/api/client.js';

export interface ChainNode extends RelationshipPathNode {
	isCommonAncestor: boolean;
}

/**
 * The path as one chain A → … → common ancestor → … → B, each person once.
 * `pathFromA` runs from A up to the ancestor and `pathFromB` from B up to it,
 * so the ancestor ends both lists.
 */
export function relationshipChain(path: RelationshipPath): ChainNode[] {
	const nodes: RelationshipPathNode[] = [];
	for (const node of [...(path.pathFromA ?? []), ...(path.pathFromB ?? []).slice().reverse()]) {
		if (nodes[nodes.length - 1]?.id !== node.id) nodes.push(node);
	}
	return nodes.map((node) => ({ ...node, isCommonAncestor: node.id === path.commonAncestorId }));
}

const GENDERED_ENDINGS: [string, string, string][] = [
	['parent', 'father', 'mother'],
	['child', 'son', 'daughter'],
	['sibling', 'brother', 'sister']
];

/**
 * The relationship name for someone of the given gender, e.g. "parent" →
 * "mother", "great-grand-uncle/aunt" → "great-grand-aunt". Names without a
 * gendered form (cousins) and unknown genders are returned unchanged.
 */
export function genderedRelationship(name: string, gender?: string): string {
	if (gender !== 'male' && gender !== 'female') return name;
	const male = gender === 'male';
	const pair = name.match(/^(.*?)(\w+)\/(\w+)$/);
	if (pair) return pair[1] + (male ? pair[2] : pair[3]);
	for (const [ending, maleForm, femaleForm] of GENDERED_ENDINGS) {
		if (name.endsWith(ending)) return name.slice(0, -ending.length) + (male ? maleForm : femaleForm);
	}
	return name;
}

/** Relationship names that differ from the primary one, each listed once. */
export function additionalRelationships(names: string[]): string[] {
	const [primary, ...rest] = names;
	return [...new Set(rest)].filter((name) => name !== primary);
}

/**
 * The relationship as a phrase addressed to the home person ("me", #1034), e.g.
 * "Your 1st cousin twice removed", with `result` calculated from the home
 * person to the person of `gender`. Only the closest relationship is named
 * (paths come closest first); further ones are counted, as the calculator
 * lists them all. Returns null when the two are not related.
 */
export function relationshipToYou(
	result: Pick<RelationshipResult, 'paths'>,
	gender?: string
): string | null {
	const names = (result.paths ?? []).map((path) => path.name ?? '').filter(Boolean);
	if (names.length === 0 || names[0] === 'self') return null;
	const phrase = `Your ${genderedRelationship(names[0], gender)}`;
	const others = additionalRelationships(names).length;
	if (others > 0) return `${phrase}, and ${others} other relationship${others === 1 ? '' : 's'}`;
	if (names.length > 1) return `${phrase} (via ${names.length} paths)`;
	return phrase;
}
