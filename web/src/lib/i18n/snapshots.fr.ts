// La page Snapshots et le comptage des fichiers (spec v0.2 §5.3, §5.7).
export default {
	'nav.snapshots': 'Snapshots',
	'snapshots.title': 'Snapshots',
	'snapshots.subtitle':
		"Une photo des fichiers d'un conteneur : chemins, tailles, dates, droits et une empreinte à clé de chaque fichier, gardés par l'agent. Le contenu des fichiers n'est jamais copié.",
	'snapshots.readOnly': "Permis en lecture seule : une photo s'écrit dans le dossier de l'agent, jamais dans Docker.",
	'snapshots.refresh': 'Actualiser',
	'snapshots.loading': 'Lecture des snapshots…',
	'snapshots.empty': 'Aucun snapshot.',
	'snapshots.take': 'Prendre une photo',
	'snapshots.take.container': 'Conteneur',
	'snapshots.take.pick': 'Choisir un conteneur',
	'snapshots.take.full': 'Photo complète',
	'snapshots.take.index': 'Photo des métadonnées',
	'snapshots.take.indexHint':
		"Tirée de l'index déjà construit : sans export ni verrou, mais sans empreinte de contenu.",
	'snapshots.take.loading': 'Lecture du conteneur…',
	'snapshots.confirm.running': 'Photographier ce conteneur actif ?',
	'snapshots.confirm.stopped': 'Photographier ce conteneur arrêté ?',
	'snapshots.confirm.index': 'Photographier les métadonnées de ce conteneur ?',
	'snapshots.cost.running':
		"Le conteneur tourne : la photo n'est pas atomique (un fichier modifié pendant le parcours peut être vu avant ou après), et jusqu'à 1 Gio est lu sur le disque, un fichier à la fois, pendant 10 min au plus.",
	'snapshots.cost.stopped':
		"Le conteneur est arrêté : Docker l'exporte en entier, volumes compris (jusqu'à 4 Gio lus), et le verrouille pendant l'export, 5 min au plus : démarrage, inspect, logs et écritures attendent.",
	'snapshots.cost.index':
		"Tirée de l'index déjà construit : aucun export, aucun verrou, aucune empreinte. La comparaison ne verra que tailles, dates, droits, liens, ajouts et suppressions.",
	'snapshots.cost.memory': "Mémoire de l'agent : environ 150 Mo au pire (500 000 entrées).",
	'snapshots.cost.size': 'Taille du conteneur selon Docker : {size}.',
	'snapshots.progress': 'Photo de {name} en cours : {n} entrées, {p} %',
	'snapshots.progress.count': 'Comptage de {name} en cours : {n} entrées',
	'snapshots.last.ok': 'Photo prise.',
	'snapshots.last.error': 'La dernière photo a échoué : {error}',
	'snapshots.col.date': 'Prise le',
	'snapshots.col.source': 'Source',
	'snapshots.col.state': 'État',
	'snapshots.col.entries': 'Entrées',
	'snapshots.partial': 'partielle',
	'snapshots.partialHint':
		"Le budget d'empreintes (1 Gio) a été atteint, ou un dossier n'a pas été lu en entier : des fichiers sont notés sans empreinte.",
	'snapshots.gone': 'conteneur supprimé',
	'snapshots.delete': 'Supprimer',
	'snapshots.delete.title': 'Supprimer cette photo ?',
	'snapshots.delete.detail': "Le manifeste est supprimé du dossier de l'agent ; rien ne change dans le conteneur.",
	'snapshots.source.procfs': 'parcours',
	'snapshots.source.archive': 'export',
	'snapshots.source.index': 'métadonnées',
	'snapshots.state.running': 'actif',
	'snapshots.state.stopped': 'arrêté',
	'snapshots.filesNone':
		"Inconnu : ni index construit, ni snapshot, ni comptage récent. « Compter » parcourt le conteneur sans lire le contenu.",
	'snapshots.filesAt.snapshot': 'Comptés par le snapshot du {at}',
	'snapshots.filesAt.count': 'Comptés le {at}',
	'snapshots.count': 'Compter',
	'snapshots.count.title': 'Compter les fichiers de ce conteneur ?',
	'snapshots.count.running': 'Le conteneur tourne : un parcours de ses fichiers, sans lire leur contenu, 10 min au plus.',
	'snapshots.count.stopped':
		"Le conteneur est arrêté : Docker l'exporte en entier, volumes compris, et le verrouille pendant l'export, 5 min au plus.",
	'snapshots.count.busy': 'Comptage… {p} %',
	'snapshots.count.failed': 'Comptage impossible : {error}'
};
