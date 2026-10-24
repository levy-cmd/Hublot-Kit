export default {
	'imagefs.tab.detail': 'Détail',
	'imagefs.tab.files': 'Fichiers',
	'imagefs.tabs': "Vues de l'image",
	'imagefs.readOnly': "Lecture seule : les fichiers sont lus dans les couches de l'image, sans créer de conteneur.",
	'imagefs.indexing': "Lecture des couches de l'image",
	'imagefs.indexingHint':
		"Première ouverture : l'agent lit l'export de l'image (docker save) sans créer de conteneur. Les suivantes sont immédiates.",
	'imagefs.extracting': 'Lecture du fichier dans sa couche',
	'imagefs.extractingHint':
		"L'agent relit l'export de l'image jusqu'à la fin de la couche qui porte ce fichier. Le contenu reste une minute en mémoire.",
	'imagefs.tooLarge':
		"Image trop volumineuse pour être explorée (au plus 4 Gio lus, 16 Gio décompressés, 1 000 000 d'entrées, 5 minutes).",
	'imagefs.fileTooLarge':
		"Pas d'aperçu : plus de 2 Mio, ou couche trop longue à lire. Les fichiers d'une image ne se téléchargent pas.",
	'imagefs.unsupported':
		"Image illisible par l'agent : couche zstd, image multi-plateforme, ou export altéré (empreinte fausse).",
	'imagefs.noIndex': "Les fichiers se cherchent dans l'index déjà construit de l'image"
};
