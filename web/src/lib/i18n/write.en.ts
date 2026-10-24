export default {
	'write.edit': 'Edit',
	'write.save': 'Save…',
	'write.cancel': 'Cancel',
	'write.saving': 'Saving…',
	'write.saved': 'Saved.',
	'write.editorLoading': 'Loading the editor…',
	'write.editorFailed': 'The editor could not load. Reload the page.',
	'write.notEditable': 'Cannot edit here: the file is not UTF-8, or it mixes CRLF and LF line endings. Download it.',
	'write.upload': 'Upload a file here',
	'write.uploading': 'Uploading…',
	'write.conflictEdit': 'The file changed since you opened it. Copy your changes, then reload it: they would be lost.',
	'write.conflictUpload': 'A file, folder or link already has this name here. Rename the file before uploading it.',
	'write.readOnly': 'Write refused: the target is read-only',
	'write.linkInPath': 'This path goes through a symbolic link: the agent only writes on a path without links. Open the link target.',
	'write.editTooLarge': 'Over 2 MiB: the editor stops there.',
	'write.invalidName': 'This name contains a quote or a line break: rename the file before uploading it.',
	'write.uploadTooLarge': 'Over 32 MiB: use docker cp for this file.',
	'write.forbidden':
		'Write refused here: Docker does not write there (mounted file or tmpfs, /etc/hosts, /dev, /proc, /sys), or a mount of the container exposes the host',
	'write.maybeWritten': 'The agent timed out, but the write may have happened. Reload the file before trying again.',
	'write.maybeWrittenNetwork':
		'Connection lost during the write: it may have happened. Reload to check before trying again.'
};
