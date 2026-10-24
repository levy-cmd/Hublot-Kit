export default {
	'write.edit': 'Modifier',
	'write.save': 'Enregistrer…',
	'write.cancel': 'Annuler',
	'write.saving': 'Enregistrement…',
	'write.saved': 'Enregistré.',
	'write.editorLoading': "Chargement de l'éditeur…",
	'write.editorFailed': "L'éditeur n'a pas pu se charger. Rechargez la page.",
	'write.notEditable':
		"Modification impossible ici : le fichier n'est pas en UTF-8, ou mélange les fins de ligne CRLF et LF. Téléchargez-le.",
	'write.upload': 'Envoyer un fichier ici',
	'write.uploading': 'Envoi…',
	'write.conflictEdit':
		'Le fichier a changé depuis son ouverture. Copiez vos modifications, puis rechargez-le : elles seraient perdues.',
	'write.conflictUpload': "Un fichier, un dossier ou un lien porte déjà ce nom ici. Renommez le fichier avant de l'envoyer.",
	'write.readOnly': 'Écriture refusée : la cible est en lecture seule',
	'write.linkInPath':
		"Ce chemin passe par un lien symbolique : l'agent n'écrit que sur un chemin sans lien. Ouvrez la cible du lien.",
	'write.editTooLarge': "Plus de 2 Mio : l'éditeur s'arrête là.",
	'write.invalidName': "Ce nom contient un guillemet ou un saut de ligne : renommez le fichier avant de l'envoyer.",
	'write.uploadTooLarge': 'Plus de 32 Mio : utilisez docker cp pour ce fichier.',
	'write.forbidden':
		"Écriture refusée à cet endroit : Docker n'y écrit pas (fichier ou tmpfs monté, /etc/hosts, /dev, /proc, /sys), ou un montage du conteneur expose l'hôte",
	'write.maybeWritten':
		"L'agent a dépassé son délai, mais l'écriture a peut-être eu lieu. Rechargez le fichier avant de réessayer.",
	'write.maybeWrittenNetwork':
		"Connexion perdue pendant l'écriture : elle a peut-être eu lieu. Rechargez pour vérifier avant de réessayer."
};
