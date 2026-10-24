export default {
	'diff.title': 'Content diff',
	'diff.legend.modified': 'Line numbers: left in the image, right in the container; + line added, − line removed.',
	'diff.legend.added': 'File added since the image: only the container version exists.',
	'diff.legend.deleted': 'File deleted since the image: only the image version exists.',
	'diff.openFile': 'Open in the explorer',
	'diff.show': 'Show the diff of {path}',
	'diff.loadingImage': 'Reading the file in the image',
	'diff.loadingContainer': 'Indexing the stopped container',
	'diff.loadingHint':
		'The agent reads the image export, or the archive of the stopped container, once. The diff shows at the end.',
	'diff.equal.bytes': 'Same content as in the image: only the metadata (mode, owner, date) changed.',
	'diff.equal.eol': 'Same text, different line endings (CRLF and LF).',
	'diff.coarse':
		'Too many differences for a line-by-line diff: everything between the common start and end is shown as one replaced block.',
	'diff.reason.binary.image': 'No diff: the image version is a binary file.',
	'diff.reason.binary.container': 'No diff: the container version is a binary file.',
	'diff.reason.too_large.image': 'No diff: the image version is over 2 MiB, or its layer takes too long to read.',
	'diff.reason.too_large.container': 'No diff: the container version is over 2 MiB.',
	'diff.reason.not_regular.image': 'No diff: in the image, this path is not a regular file (directory, link, device…).',
	'diff.reason.not_regular.container': 'No diff: in the container, this path is not a regular file (directory, link, device…).',
	'diff.reason.missing.image': 'No diff: this path does not exist in the image.',
	'diff.reason.missing.container': 'No diff: this path no longer exists in the container.'
};
