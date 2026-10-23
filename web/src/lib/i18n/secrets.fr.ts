// The Secrets page (spec v0.2 §5.9).
export default {
	'nav.secrets': 'Secrets',
	'secrets.title': 'Secrets',
	'secrets.subtitle':
		"Les noms des variables, arguments et labels que l'agent masque, conteneur par conteneur. Aucune valeur n'est envoyée sans une révélation journalisée.",
	'secrets.refresh': 'Actualiser',
	'secrets.loading': 'Inventaire en cours…',
	'secrets.project': 'Projet',
	'secrets.project.any': 'Tous les projets',
	'secrets.project.none': 'Sans projet',
	'secrets.summary': '{secrets} secrets détectés dans {containers} conteneurs sur {total}',
	'secrets.empty': 'Aucun conteneur.',
	'secrets.unavailable': 'indisponible',
	'secrets.unavailableHint':
		"Docker n'a pas répondu en 3 s (conteneur verrouillé par un export, par exemple). Actualiser pour réessayer.",
	'secrets.none': 'aucun secret détecté',
	'secrets.count.env': '{n} var.',
	'secrets.count.args': '{n} arg.',
	'secrets.count.labels': '{n} labels masqués',
	'secrets.env': 'Variables',
	'secrets.args': 'Arguments masqués',
	'secrets.argsHint': 'La commande ne se révèle pas : elle est montrée telle que la radiographie la masque.',
	'secrets.labels': 'Labels masqués',
	'secrets.labelsHint':
		'Tout label hors de com.docker.compose.* et org.opencontainers.image.* est masqué par précaution, secret ou non. Les labels ne se révèlent pas.',
	'secrets.revealHint': 'Une valeur révélée est journalisée (env.reveal) et oubliée en quittant la page.',
	'secrets.open': 'Ouvrir le conteneur',
	'secrets.masked': 'masquée',
	'secrets.reveal': 'Révéler',
	'secrets.hide': 'Masquer'
};
