import { describe, it, expect, vi, afterEach, type Mock } from 'vitest';
import { useShortcuts } from './useShortcuts.svelte';
import { DEFAULT_SHORTCUTS, type ShortcutContext } from './shortcuts';

function press(handleKeydown: (event: KeyboardEvent) => void, key: string, init: KeyboardEventInit = {}) {
	const event = new KeyboardEvent('keydown', { key, cancelable: true, ...init });
	handleKeydown(event);
	return event;
}

/** A handler spy for every action the context's shortcuts name. */
function spiesFor(context: ShortcutContext): Record<string, Mock<() => void>> {
	const actions = DEFAULT_SHORTCUTS.filter((s) => s.context === context).map((s) => s.action);
	return Object.fromEntries(actions.map((action) => [action, vi.fn<() => void>()]));
}

describe('useShortcuts', () => {
	afterEach(() => {
		document.body.innerHTML = '';
	});

	it.each(['ArrowUp', 'Enter'])('fires a registered %s shortcut on the matching KeyboardEvent', (key) => {
		const handlers = spiesFor('pedigree');
		const { handleKeydown, cleanup } = useShortcuts('pedigree', handlers);

		const event = press(handleKeydown, key);

		const action = DEFAULT_SHORTCUTS.find((s) => s.context === 'pedigree' && s.keys[0] === key)!.action;
		expect(handlers[action]).toHaveBeenCalledTimes(1);
		expect(event.defaultPrevented).toBe(true);
		cleanup();
	});

	// Every key the ? dialog lists under a chart group must fire its action.
	describe.each<ShortcutContext>(['pedigree', 'descendancy'])('%s chart group', (context) => {
		const shortcuts = DEFAULT_SHORTCUTS.filter((s) => s.context === context);

		it.each(shortcuts.map((s) => [s.keys.join(' '), s.action, s.keys] as const))(
			'%s fires %s',
			(_label, action, keys) => {
				const handlers = spiesFor(context);
				const { handleKeydown, cleanup } = useShortcuts(context, handlers);

				for (const key of keys) press(handleKeydown, key);

				expect(handlers[action]).toHaveBeenCalledTimes(1);
				for (const [other, spy] of Object.entries(handlers)) {
					if (other !== action) expect(spy).not.toHaveBeenCalled();
				}
				cleanup();
			}
		);
	});

	it('still matches letter shortcuts typed with Shift', () => {
		const handlers = { 'reset-view': vi.fn() };
		const { handleKeydown, cleanup } = useShortcuts('pedigree', handlers);

		press(handleKeydown, 'R', { shiftKey: true });

		expect(handlers['reset-view']).toHaveBeenCalledTimes(1);
		cleanup();
	});

	it('does not match a named key by its lower-cased spelling', () => {
		const handlers = { 'navigate-father': vi.fn() };
		const { handleKeydown, cleanup } = useShortcuts('pedigree', handlers);

		press(handleKeydown, 'arrowup');

		expect(handlers['navigate-father']).not.toHaveBeenCalled();
		cleanup();
	});

	it('ignores arrow keys while an input has focus', () => {
		const handlers = { 'navigate-father': vi.fn() };
		const { handleKeydown, cleanup } = useShortcuts('pedigree', handlers);
		const input = document.createElement('input');
		document.body.appendChild(input);
		input.focus();

		const event = press(handleKeydown, 'ArrowUp');

		expect(handlers['navigate-father']).not.toHaveBeenCalled();
		expect(event.defaultPrevented).toBe(false);
		cleanup();
	});

	it.each([
		['button', () => document.createElement('button')],
		['link', () => Object.assign(document.createElement('a'), { href: '/persons' })],
		[
			'scroll region',
			() => {
				const region = document.createElement('div');
				region.setAttribute('role', 'region');
				region.tabIndex = 0;
				return region;
			}
		]
	])('leaves Enter and arrow keys to a focused %s', (_name, create) => {
		const handlers = spiesFor('pedigree');
		const { handleKeydown, cleanup } = useShortcuts('pedigree', handlers);
		const control = create();
		document.body.appendChild(control);
		control.focus();

		const enter = press(handleKeydown, 'Enter');
		const arrow = press(handleKeydown, 'ArrowUp');

		for (const spy of Object.values(handlers)) expect(spy).not.toHaveBeenCalled();
		expect(enter.defaultPrevented).toBe(false);
		expect(arrow.defaultPrevented).toBe(false);
		cleanup();
	});

	it('still fires letter shortcuts while a button has focus', () => {
		const handlers = { 'reset-view': vi.fn() };
		const { handleKeydown, cleanup } = useShortcuts('pedigree', handlers);
		const button = document.createElement('button');
		document.body.appendChild(button);
		button.focus();

		press(handleKeydown, 'r');

		expect(handlers['reset-view']).toHaveBeenCalledTimes(1);
		cleanup();
	});
});
