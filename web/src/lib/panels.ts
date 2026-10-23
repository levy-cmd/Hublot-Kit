// The bottom panels of a container (spec v0.2 §3).
export type Panel = 'files' | 'diff' | 'logs';
export const PANELS: readonly Panel[] = ['files', 'diff', 'logs'];

/**
 * The panels to mount. From 90 rem (wide) they are side by side: all of
 * them. Below, one shows at a time: only the panels opened so far, and the
 * current one. A hidden file system panel would otherwise start the export
 * of a stopped container (up to 4 GiB and 5 min, the container locked by
 * dockerd) for a panel never shown; a panel stays mounted once opened, so
 * that its logs stream or its index is not started again.
 */
export function mountedPanels(opened: ReadonlySet<Panel>, current: Panel, wide: boolean): Set<Panel> {
	return wide ? new Set(PANELS) : new Set([...opened, current]);
}
