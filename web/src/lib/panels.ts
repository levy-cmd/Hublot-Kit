// The bottom panels of a container (spec v0.2 §3).
export type Panel = 'files' | 'diff' | 'logs';
export const PANELS: readonly Panel[] = ['files', 'diff', 'logs'];

/**
 * From this width the four panels are side by side ("wide"); below, tabs.
 * 110 rem (1760 px), not 90: at 1440 px the diff had 217 px, its lines cut
 * (decision of 2026-10-25). The style of BottomPanels.svelte repeats it in
 * its @media queries; panels.test.ts checks that both agree.
 */
export const WIDE_QUERY = '(min-width: 110rem)';

/**
 * The panels to mount. From 110 rem (wide) they are side by side: all of
 * them. Below, one shows at a time: only the panels opened so far, and the
 * current one. A hidden file system panel would otherwise start the export
 * of a stopped container (up to 4 GiB and 5 min, the container locked by
 * dockerd) for a panel never shown; a panel stays mounted once opened, so
 * that its index is not built again, nor its lines lost.
 */
export function mountedPanels(opened: ReadonlySet<Panel>, current: Panel, wide: boolean): Set<Panel> {
	return wide ? new Set(PANELS) : new Set([...opened, current]);
}

/**
 * Whether the logs panel keeps its stream open. Below 110 rem a hidden logs
 * panel stays mounted (its lines stay) but closes its stream, which would
 * hold one of the agent's stream slots for nothing; shown again, it resumes
 * after the last line received (Last-Event-ID).
 */
export const logsLive = (current: Panel, wide: boolean): boolean => wide || current === 'logs';
