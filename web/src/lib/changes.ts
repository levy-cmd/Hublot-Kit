import type { Change } from './api.ts';
import { t } from './i18n/index.ts';

type ChangeKind = Change['kind'];

/** Changes listed by the Changes tab; the rest is only counted (Q12). */
export const CHANGES_SHOWN = 1000;

export function changeMarks(changes: Change[]): Map<string, ChangeKind> {
	return new Map(changes.map((c) => [c.path, c.kind]));
}

/** Names of the deleted direct children of dir (not deeper descendants). */
export function deletedIn(changes: Change[], dir: string): string[] {
	const prefix = dir === '/' ? '/' : dir + '/';
	const names: string[] = [];
	for (const c of changes) {
		if (c.kind !== 'deleted' || !c.path.startsWith(prefix)) continue;
		const rest = c.path.slice(prefix.length);
		if (rest !== '' && !rest.includes('/')) names.push(rest);
	}
	return names;
}

export function countChanges(changes: Change[]): Record<ChangeKind, number> {
	const n: Record<ChangeKind, number> = { added: 0, modified: 0, deleted: 0 };
	for (const c of changes) n[c.kind]++;
	return n;
}

export const kindLetter = (kind: ChangeKind): 'A' | 'M' | 'D' =>
	kind === 'added' ? 'A' : kind === 'modified' ? 'M' : 'D';

export const kindLabel = (kind: ChangeKind): string => t(`changes.${kind}`);
