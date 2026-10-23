export default {
	'imagefs.tab.detail': 'Detail',
	'imagefs.tab.files': 'Files',
	'imagefs.tabs': 'Image views',
	'imagefs.readOnly': 'Read only: the files are read from the layers of the image, no container is created.',
	'imagefs.indexing': 'Reading the layers of the image',
	'imagefs.indexingHint':
		'First opening: the agent reads the export of the image (docker save) without creating a container. The next ones are immediate.',
	'imagefs.extracting': 'Reading the file in its layer',
	'imagefs.extractingHint':
		'The agent reads the export of the image again, up to the end of the layer that holds this file. The content stays in memory for a minute.',
	'imagefs.tooLarge':
		'Image too large to explore (at most 4 GiB read, 16 GiB decompressed, 1,000,000 entries, 5 minutes).',
	'imagefs.fileTooLarge':
		'No preview: over 2 MiB, or a layer too long to read. The files of an image cannot be downloaded.',
	'imagefs.unsupported':
		'The agent cannot read this image: zstd layer, multi-platform image, or altered export (wrong digest).',
	'imagefs.noIndex': 'Files are searched in the index of the image already built'
};
