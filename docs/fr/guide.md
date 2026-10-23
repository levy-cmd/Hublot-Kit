# Visite de l'interface

Cette page décrit l'interface de Hublot-Kit à partir de la v0.2. Pour l'installer, voir [Installation](installation.md) ; pour ce que l'agent peut faire et ce qui le protège, voir [le modèle de sécurité](securite.md).

## La barre du haut

- À gauche, l'anneau du hublot, le nom et la version réelle de l'agent (`dev` pour une version de développement).
- Au centre, le champ « Rechercher un conteneur, une image, un fichier… » avec l'indication `Ctrl K` (`⌘ K` sur macOS). Il ouvre la recherche (ci-dessous) et ne cherche rien par lui-même.
- À droite :
  - l'état de Docker : « Connecté » ou « Docker injoignable », suivi de l'adresse du démon (par exemple `unix:///var/run/docker.sock`). Une adresse qui contiendrait un identifiant (`tcp://utilisateur:motdepasse@hôte`) est masquée par l'agent ;
  - le bouclier : plein avec « Lecture seule » quand l'agent a démarré avec `--read-only`, en contour avec « Écriture permise » sinon ;
  - la bascule du thème ;
  - le menu du serveur : l'adresse de l'agent, « Serveurs », « Paramètres » et « Se déconnecter ». Hublot-Kit n'a pas de comptes, seulement un jeton : le menu est celui du serveur, pas d'une personne.

## La barre latérale

Elle mène aux pages qui existent : Conteneurs (avec leur nombre), Images (avec leur nombre), Diff avec l'image, Recherche, Logs, Secrets et Paramètres. « Recherche » ouvre la recherche globale. « Diff avec l'image » et « Logs » ouvrent le panneau correspondant du conteneur choisi ; sans conteneur choisi, la liste demande d'en choisir un.

En bas, un encart dit l'état réel de l'agent :

- « Lecture seule : aucune écriture dans les conteneurs » quand l'agent a démarré avec `--read-only` ;
- « Écriture permise, toujours confirmée » sinon. C'est le cas par défaut : chaque écriture passe par une boîte de confirmation qui nomme le conteneur et le chemin.

Sous 80 rem de large, la barre se réduit à ses icônes ; sous 48 rem, elle devient un tiroir qu'ouvre le bouton de menu de la barre du haut.

## La recherche (Ctrl+K)

`Ctrl K` (`⌘ K` sur macOS), le champ de la barre du haut ou l'entrée « Recherche » ouvrent une palette, sur toutes les pages sauf celle de connexion.

- Elle cherche les conteneurs (nom, ID court, image, projet, service), les projets Compose et les pages de l'interface, dans la liste que le navigateur a déjà : rien n'est demandé à l'agent pour cela. Un nom exact passe en premier, puis un début de nom, puis un nom qui contient la recherche ; un conteneur passe avant un projet ou une page.
- Elle cherche aussi les **fichiers**, mais seulement ceux du conteneur ouvert (`?c=` ou la page du conteneur), et seulement dans l'index déjà construit quand l'explorateur l'a ouvert. La recherche ne lance jamais d'export ni de parcours : pour un conteneur en marche, ou un conteneur arrêté que l'explorateur n'a pas encore ouvert, la palette propose « Ouvrir l'explorateur ». La recherche porte sur le chemin complet, sans tenir compte de la casse ; les noms qui contiennent la recherche passent avant les chemins qui la contiennent par un dossier parent. 200 fichiers au plus, puis la palette demande de préciser.
- Elle ne renvoie que des noms que l'explorateur montre déjà pour ce conteneur, jamais un contenu, jamais une valeur masquée (environnement, commande, labels).
- Au clavier : les flèches choisissent, Entrée ouvre, Échap ferme et rend le focus là où il était ; Tab reste dans la palette.
- Rien n'est gardé : ni historique, ni stockage du navigateur, ni requête dans l'adresse de la page. La requête des fichiers part dans le corps d'un appel POST à l'agent (`/api/containers/{id}/fs/search`), jamais dans une adresse ; l'agent ne la journalise pas (voir [les limites](limites.md)).

Les images rejoindront la palette avec la page Images.

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

Les labels portent souvent des secrets que le masquage ne reconnaît pas, par exemple `traefik.http.middlewares.auth.basicauth.users=admin:$apr1$…`. Seules les valeurs des labels `com.docker.compose.*` et `org.opencontainers.image.*` sont montrées, et seulement si le masquage n'y trouve rien ; celles qui donnent des chemins de la machine hôte (`com.docker.compose.project.config_files`, `.working_dir` et `.environment_file`) sont masquées aussi. Les valeurs montrées forment des puces ; les labels masqués tiennent sur une ligne repliée, « N autres labels masqués », qui ne déplie que leurs noms. Aucun label ne se révèle : sa valeur se lit sur l'hôte avec `docker inspect`.

## Les panneaux du bas

À partir de 90 rem de large, quatre panneaux côte à côte ; en dessous, des onglets.

1. **Système de fichiers** : l'explorateur. « Explorer » montre tout le dossier, « Diff » seulement ce qui a changé depuis l'image ; dans les deux cas, une lettre marque un fichier ajouté (A) ou modifié (M). Pour un conteneur arrêté, la première ouverture construit un index, avec sa progression.
2. **Diff avec l'image** : les compteurs des ajouts, modifications et suppressions, puis la liste ; un chemin ouvre le fichier dans l'explorateur.
3. **Contenu du fichier** : l'aperçu avec ses numéros de ligne, le téléchargement et, si l'écriture est permise, la modification.
4. **Logs en direct** : l'horodatage, le niveau et le message. Le niveau est déduit du texte (`ERROR`, `WARN`, `INFO`, `DEBUG`, ou un champ `level`) ; c'est une heuristique, et à défaut le flux d'origine (`stdout` ou `stderr`) est affiché. « Suivi automatique » fait suivre la fin du flux ; le couper ne met pas le flux en pause. « Pause » arrête la réception, « Reprendre » rattrape les lignes arrivées entre-temps.

## La page Images

La page Images liste les images du démon, celles sans étiquette comprises (montrées par leur ID court), avec leur taille, leur date et les conteneurs qui les utilisent. Elle se trie par date, par taille ou par nom ; le champ filtre par étiquette, par début d'ID ou par nom de conteneur. Les images intermédiaires de l'ancien constructeur ne sont pas listées.

Une image s'ouvre sur sa page, `/images/<ID>`. Elle se désigne toujours par son **ID complet** (64 caractères hexadécimaux), jamais par un nom comme `nginx:latest` : un nom peut changer de cible entre deux requêtes. Une adresse avec un nom ou un ID court répond « Introuvable ». Une image dé-étiquetée (`docker rmi -f` d'une image utilisée) reste donc lisible tant qu'elle existe.

La page montre :

- l'ID complet (avec copie), les étiquettes, la taille, la date et la plateforme (`linux/arm64/v8`) ;
- la configuration : la commande, le dossier de travail, l'utilisateur, les ports exposés, les variables et les labels, masqués par les mêmes règles que la radiographie d'un conteneur. Une valeur masquée ne se révèle pas depuis l'image : la révélation, journalisée, se fait depuis un conteneur qui l'utilise ;
- les couches, c'est-à-dire l'historique de construction, de la plus récente à la plus ancienne. Les entrées sans fichier (`ENV`, `CMD`, `LABEL`…) sont repliées. Les arguments de construction (`|2 GH_PAT=… /bin/sh -c …`, `ARG GH_PAT=…`) sont masqués quel que soit leur nom, et une ligne `LABEL` ne garde aucun texte : les labels se lisent dans la configuration, filtrés ;
- les conteneurs de l'image, qui mènent à leur détail.

La liste coûte deux demandes à Docker, quel que soit le nombre d'images (la liste des images et celle des conteneurs) ; deux onglets ouverts en même temps partagent la même lecture. Le détail en coûte trois. Si Docker ne répond pas en 10 s, la page le dit et « Actualiser » réessaie. Les fichiers d'une image ne sont pas encore explorables.

## La page Secrets

La page Secrets fait l'inventaire de ce que l'agent masque, sur tous les conteneurs à la fois. Elle ne montre que des **noms**, jamais une valeur :

- les variables d'environnement que la radiographie masque ;
- les arguments de la commande et du point d'entrée où l'agent a trouvé un secret, par leur place et par ce qui a été détecté (« Commande, argument 3 : valeur de --requirepass »), jamais par leur texte : un masque partiel pourrait laisser voir un reste de la valeur ;
- les noms des labels masqués. Tout label hors de `com.docker.compose.*` et `org.opencontainers.image.*` est masqué par précaution, qu'il porte un secret ou non : ils sont comptés à part.

Les conteneurs sont triés par nombre de secrets détectés (variables et arguments), puis par nombre de labels masqués ; la liste « Projet » filtre par projet Compose, comme sur la page Conteneurs. Une ligne se déplie sur ses variables ; « Révéler » passe par la même demande que l'onglet Secrets du détail, journalisée par l'agent, et permise en lecture seule. La commande et les labels ne se révèlent pas.

Une valeur révélée n'est gardée que par la page : elle est oubliée quand on replie la ligne, qu'on actualise, qu'on quitte la page ou qu'on ferme l'onglet. Rien n'est écrit dans le stockage du navigateur.

L'inventaire coûte un `docker inspect` par conteneur (quelques millisecondes), quatre à la fois au plus pour tout l'agent. Un conteneur qui ne répond pas en 3 s, par exemple parce que Docker le verrouille pendant son export, est marqué « indisponible » sans retenir les autres ; « Actualiser » réessaie. Rien n'est gardé en cache : la page relit Docker à chaque ouverture.

## La page d'un conteneur et le téléphone

Chaque conteneur a sa page, `/c/<identifiant>`, avec les onglets Aperçu, Logs, Diff, Fichiers et Secrets. C'est aussi ce qu'ouvre un téléphone : la liste, puis le détail en page pleine. Les anciens liens de la v0.1 (`?tab=changes`, `?tab=xray`) mènent toujours au bon onglet. La modification de fichiers reste pensée pour un ordinateur.

## Le thème

Sombre par défaut, même si le système demande le clair. La bascule de la barre du haut, ou la page Paramètres, passe au thème clair ; le choix est gardé dans ce navigateur seulement (`localStorage`, clé `hublot-theme`) et appliqué avant le premier affichage.

## Les paramètres

La page Paramètres montre la langue (celle du navigateur) et le thème, puis, en lecture seule, la version de l'agent, la lecture seule et Docker : adresse, état, version du démon et version d'API. Ces valeurs sont des options de démarrage de l'agent ; elles ne se modifient pas depuis l'interface.
