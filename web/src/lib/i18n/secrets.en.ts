// The Secrets page (spec v0.2 §5.9).
export default {
	'nav.secrets': 'Secrets',
	'secrets.title': 'Secrets',
	'secrets.subtitle':
		'The names of the variables, arguments and labels the agent masks, container by container. No value is sent without a journaled reveal.',
	'secrets.refresh': 'Refresh',
	'secrets.loading': 'Taking the inventory…',
	'secrets.project': 'Project',
	'secrets.project.any': 'All projects',
	'secrets.project.none': 'No project',
	'secrets.summary': '{secrets} secrets detected in {containers} of {total} containers',
	'secrets.empty': 'No container.',
	'secrets.unavailable': 'unavailable',
	'secrets.unavailableHint':
		'Docker did not answer within 3 s (a container locked by an export, for instance). Refresh to try again.',
	'secrets.none': 'no secret detected',
	'secrets.count.env': '{n} var.',
	'secrets.count.args': '{n} arg.',
	'secrets.count.labels': '{n} masked labels',
	'secrets.env': 'Variables',
	'secrets.args': 'Masked arguments',
	'secrets.argsHint': 'The command is not revealed: it is shown as the radiography masks it.',
	'secrets.labels': 'Masked labels',
	'secrets.labelsHint':
		'Every label outside com.docker.compose.* and org.opencontainers.image.* is masked as a precaution, secret or not. Labels are not revealed.',
	'secrets.revealHint': 'A revealed value is journaled (env.reveal) and forgotten when you leave the page.',
	'secrets.open': 'Open the container',
	'secrets.masked': 'masked',
	'secrets.reveal': 'Reveal',
	'secrets.hide': 'Hide'
};
