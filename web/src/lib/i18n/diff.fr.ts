export default {
	'diff.title': 'Diff de contenu',
	'diff.legend.modified': "Numéros de ligne : à gauche dans l'image, à droite dans le conteneur ; + ligne ajoutée, − ligne retirée.",
	'diff.legend.added': "Fichier ajouté depuis l'image : seule la version du conteneur existe.",
	'diff.legend.deleted': "Fichier supprimé depuis l'image : seule la version de l'image existe.",
	'diff.openFile': "Ouvrir dans l'explorateur",
	'diff.show': 'Voir le diff de {path}',
	'diff.loadingImage': "Lecture du fichier dans l'image",
	'diff.loadingContainer': 'Indexation du conteneur arrêté',
	'diff.loadingHint':
		"L'agent relit l'export de l'image, ou l'archive du conteneur arrêté, une seule fois. Le diff s'affiche à la fin.",
	'diff.equal.bytes': "Contenu identique à celui de l'image : seules les métadonnées (droits, propriétaire, date) ont changé.",
	'diff.equal.eol': 'Même texte, fins de ligne différentes (CRLF et LF).',
	'diff.coarse':
		'Trop de différences pour un diff ligne à ligne : tout ce qui se trouve entre le début et la fin communs est montré en un seul bloc remplacé.',
	'diff.reason.binary.image': "Pas de diff : la version de l'image est un fichier binaire.",
	'diff.reason.binary.container': 'Pas de diff : la version du conteneur est un fichier binaire.',
	'diff.reason.too_large.image': "Pas de diff : la version de l'image dépasse 2 Mio, ou sa couche est trop longue à lire.",
	'diff.reason.too_large.container': 'Pas de diff : la version du conteneur dépasse 2 Mio.',
	'diff.reason.not_regular.image': "Pas de diff : dans l'image, ce chemin n'est pas un fichier régulier (dossier, lien, périphérique…).",
	'diff.reason.not_regular.container':
		"Pas de diff : dans le conteneur, ce chemin n'est pas un fichier régulier (dossier, lien, périphérique…).",
	'diff.reason.missing.image': "Pas de diff : ce chemin n'existe pas dans l'image.",
	'diff.reason.missing.container': "Pas de diff : ce chemin n'existe plus dans le conteneur."
};
