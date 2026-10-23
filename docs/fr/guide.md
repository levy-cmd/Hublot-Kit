# Visite de l'interface

Cette page décrit l'interface de Hublot-Kit à partir de la v0.2. Pour l'installer, voir [Installation](installation.md) ; pour ce que l'agent peut faire et ce qui le protège, voir [le modèle de sécurité](securite.md).

## La barre du haut

- À gauche, l'anneau du hublot, le nom et la version réelle de l'agent (`dev` pour une version de développement).
- À droite :
  - l'état de Docker : « Connecté » ou « Docker injoignable », suivi de l'adresse du démon (par exemple `unix:///var/run/docker.sock`). Une adresse qui contiendrait un identifiant (`tcp://utilisateur:motdepasse@hôte`) est masquée par l'agent ;
  - le bouclier : plein avec « Lecture seule » quand l'agent a démarré avec `--read-only`, en contour avec « Écriture permise » sinon ;
  - la bascule du thème ;
  - le menu du serveur : l'adresse de l'agent, « Serveurs », « Paramètres » et « Se déconnecter ». Hublot-Kit n'a pas de comptes, seulement un jeton : le menu est celui du serveur, pas d'une personne.

## La barre latérale

Elle mène aux pages qui existent : Conteneurs (avec leur nombre), Diff avec l'image, Logs et Paramètres. « Diff avec l'image » et « Logs » ouvrent le panneau correspondant du conteneur choisi ; sans conteneur choisi, la liste demande d'en choisir un.

En bas, un encart dit l'état réel de l'agent :

- « Lecture seule : aucune écriture dans les conteneurs » quand l'agent a démarré avec `--read-only` ;
- « Écriture permise, toujours confirmée » sinon. C'est le cas par défaut : chaque écriture passe par une boîte de confirmation qui nomme le conteneur et le chemin.

Sous 80 rem de large, la barre se réduit à ses icônes ; sous 48 rem, elle devient un tiroir qu'ouvre le bouton de menu de la barre du haut.

## La page Conteneurs

- Les onglets Tous, En cours et Arrêtés filtrent la liste ; un conteneur en pause ou qui redémarre compte parmi « En cours ». La liste « Statut » choisit un état précis de Docker.
- Le champ de filtre cherche dans le nom et l'image. Il n'est pas gardé dans l'adresse de la page, car un nom peut être sensible ; l'onglet, le statut et le projet le sont.
- La colonne Projet et la liste « Projet » viennent des labels de Compose (`com.docker.compose.project` et `.service`). La liste propose les projets présents, plus « Sans projet ». Un nom de projet que le masquage des secrets refuserait n'est pas envoyé : le conteneur compte alors comme « sans projet ».
- La colonne Uptime traduit le statut de Docker : `Up 3 hours (healthy)` devient « depuis 3 heures, sain », `Exited (137) 2 days ago` devient « arrêté il y a 2 jours, code 137 ». Un texte que l'interface ne reconnaît pas est montré tel quel.
- La couleur double toujours un texte : émeraude pour un conteneur en marche, ambre pour une alerte (pause, redémarrage, sortie avec un code non nul), gris pour un arrêt propre.
- Au clavier : `/` place le curseur dans le filtre, les flèches, Début et Fin parcourent les lignes, Entrée choisit le conteneur.

## Le panneau de détail

Choisir un conteneur ouvre à droite son détail : l'image (avec un bouton de copie), la date de création, la dernière sortie (« jamais » pour un conteneur jamais arrêté), trois chiffres clés, puis deux onglets.

- **Aperçu** : la commande, sous la forme masquée par l'agent (le bouton de copie copie cette forme, jamais une valeur révélée), les ports, les labels, puis, sous « Détails », l'identité, l'état, les réseaux et les montages.
- **Secrets** : les variables d'environnement. Une valeur secrète est masquée ; « Révéler » la demande à l'agent, qui journalise la révélation. Une valeur révélée est oubliée quand on quitte l'onglet ou le conteneur.

Les boutons « Explorer les fichiers », « Comparer avec l'image » et « Voir les logs » ouvrent les panneaux du bas.

### Chiffres clés

Ils arrivent après le reste du détail, sans le retenir ; « — » veut dire en cours de chargement, inconnu ou indisponible.

- **Fichiers** : le nombre d'entrées de l'index d'un conteneur arrêté, s'il est déjà construit (en ouvrant ses fichiers). L'agent ne lance jamais une construction pour ce chiffre : sans index, et pour un conteneur en marche, il reste « — ».
- **Taille de l'image** : la taille que Docker donne pour l'image du conteneur, gardée en mémoire par image (une image ne change pas).
- **Taille du conteneur** : la couche inscriptible (ce que le conteneur a écrit depuis sa création), puis le total avec l'image. Pour la calculer, Docker parcourt la couche inscriptible : l'agent ne la demande qu'à l'ouverture du détail, jamais pour la liste, et la garde une minute. Pendant l'export d'un conteneur arrêté, Docker le verrouille : le chiffre attend, 30 s au plus.

### Labels

Les labels portent souvent des secrets que le masquage ne reconnaît pas, par exemple `traefik.http.middlewares.auth.basicauth.users=admin:$apr1$…`. Seules les valeurs des labels `com.docker.compose.*` et `org.opencontainers.image.*` sont montrées, et seulement si le masquage n'y trouve rien. Les autres labels montrent leur nom avec la valeur « masqué », sans révélation possible : leur valeur se lit sur l'hôte avec `docker inspect`.

## Les panneaux du bas

À partir de 90 rem de large, quatre panneaux côte à côte ; en dessous, des onglets.

1. **Système de fichiers** : l'explorateur. « Explorer » montre tout le dossier, « Diff » seulement ce qui a changé depuis l'image ; dans les deux cas, une lettre marque un fichier ajouté (A) ou modifié (M). Pour un conteneur arrêté, la première ouverture construit un index, avec sa progression.
2. **Diff avec l'image** : les compteurs des ajouts, modifications et suppressions, puis la liste ; un chemin ouvre le fichier dans l'explorateur.
3. **Contenu du fichier** : l'aperçu avec ses numéros de ligne, le téléchargement et, si l'écriture est permise, la modification.
4. **Logs en direct** : l'horodatage, le niveau et le message. Le niveau est déduit du texte (`ERROR`, `WARN`, `INFO`, `DEBUG`, ou un champ `level`) ; c'est une heuristique, et à défaut le flux d'origine (`stdout` ou `stderr`) est affiché. « Suivi automatique » fait suivre la fin du flux ; le couper ne met pas le flux en pause. « Pause » arrête la réception, « Reprendre » rattrape les lignes arrivées entre-temps.

## La page d'un conteneur et le téléphone

Chaque conteneur a sa page, `/c/<identifiant>`, avec les onglets Aperçu, Logs, Diff, Fichiers et Secrets. C'est aussi ce qu'ouvre un téléphone : la liste, puis le détail en page pleine. Les anciens liens de la v0.1 (`?tab=changes`, `?tab=xray`) mènent toujours au bon onglet. La modification de fichiers reste pensée pour un ordinateur.

## Le thème

Sombre par défaut, même si le système demande le clair. La bascule de la barre du haut, ou la page Paramètres, passe au thème clair ; le choix est gardé dans ce navigateur seulement (`localStorage`, clé `hublot-theme`) et appliqué avant le premier affichage.

## Les paramètres

La page Paramètres montre la langue (celle du navigateur) et le thème, puis, en lecture seule, la version de l'agent, la lecture seule et Docker : adresse, état, version du démon et version d'API. Ces valeurs sont des options de démarrage de l'agent ; elles ne se modifient pas depuis l'interface.
