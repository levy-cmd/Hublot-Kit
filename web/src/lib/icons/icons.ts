// The in-house icon set (spec v0.2 §4.3): drawn on a 20 × 20 grid, 1.5 px
// strokes kept at any size (vector-effect), no icon library. Each icon is
// one path; "M… M…" starts a new stroke.
export const ICONS = {
	container: 'M3.5 5.5h13v9h-13z M7 7.5v5 M10 7.5v5 M13 7.5v5',
	image: 'M10 3 17 6.5 10 10 3 6.5z M3 10l7 3.5 7-3.5 M3 13.5 10 17l7-3.5',
	diff: 'M4 7h11 M12 4l3 3-3 3 M16 13H5 M8 10l-3 3 3 3',
	logs: 'M4 5h12 M4 8.5h12 M4 12h8 M4 15.5h10',
	settings: 'M3.5 6h8 M14.5 6h2 M13 4v4 M3.5 14h2 M8.5 14h8 M7 12v4',
	search: 'M8.5 3.5a5 5 0 1 0 0 10a5 5 0 1 0 0-10z M12.2 12.2 16.5 16.5',
	shield: 'M10 2.8 16 5v4.6c0 3.7-2.6 6.4-6 7.6-3.4-1.2-6-3.9-6-7.6V5z',
	sun: 'M10 7a3 3 0 1 0 0 6a3 3 0 1 0 0-6z M10 2.5v2 M10 15.5v2 M2.5 10h2 M15.5 10h2 M4.7 4.7l1.4 1.4 M13.9 13.9l1.4 1.4 M4.7 15.3l1.4-1.4 M13.9 6.1l1.4-1.4',
	moon: 'M15.5 12.5A6.5 6.5 0 0 1 7.5 4.5a6.5 6.5 0 1 0 8 8z',
	refresh: 'M15.5 10a5.5 5.5 0 1 1-1.6-3.9 M15.5 3.8v3.4h-3.4',
	copy: 'M7 7h9v9H7z M4 13V4h9',
	folder: 'M3 5.5h5l1.5 1.5H17v8.5H3z',
	file: 'M5.5 3h6l3.5 3.5V17h-9.5z M11.5 3v3.5H15',
	link: 'M8 12l4-4 M9 5.5l1-1a3 3 0 0 1 4.5 4.5l-1 1 M11 14.5l-1 1a3 3 0 0 1-4.5-4.5l1-1',
	chevronDown: 'M6 8l4 4 4-4',
	chevronRight: 'M8 6l4 4-4 4',
	arrowLeft: 'M15 10H5 M9 6l-4 4 4 4',
	download: 'M10 3.5v9 M6.5 9 10 12.5 13.5 9 M4 16h12',
	upload: 'M10 12.5v-9 M6.5 7 10 3.5 13.5 7 M4 16h12',
	menu: 'M3.5 6h13 M3.5 10h13 M3.5 14h13',
	close: 'M5 5l10 10 M15 5 5 15',
	server: 'M3.5 4h13v5h-13z M3.5 11h13v5h-13z M6.5 6.5h1 M6.5 13.5h1',
	logout: 'M8 4H4v12h4 M12 6.5 15.5 10 12 13.5 M15.5 10H7.5',
	expand: 'M11.5 3.5h5v5 M16.5 3.5 11 9 M8.5 16.5h-5v-5 M3.5 16.5 9 11',
	terminal: 'M4 6l3.5 3.5L4 13 M9.5 14h6.5',
	port: 'M7 3.5v4 M13 3.5v4 M5 7.5h10V10a5 5 0 0 1-10 0z M10 15v2',
	lock: 'M5 9h10v7.5H5z M7 9V6.5a3 3 0 0 1 6 0V9',
	edit: 'M13.5 3.5l3 3L7 16H4v-3z',
	check: 'M4.5 10.5l3.5 3.5 7.5-8',
	info: 'M10 3a7 7 0 1 0 0 14a7 7 0 1 0 0-14z M10 9v4.5 M10 6.5v.5'
} as const;

export type IconName = keyof typeof ICONS;
