# Limites connues (v0.1)

La v0.1 est une première version publique. Cette page dit ce qu'elle ne fait pas, et les cas qu'elle ne détecte pas,
en connaissance de cause. La spec technique ([§13](../superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md)) donne
tous les détails.

## Périmètre

- Un serveur Docker, un utilisateur. Plusieurs serveurs ou utilisateurs : plus tard.
- Non pris en charge : Docker rootless, `userns-remap`, Podman, Kubernetes.
- Absents de la v0.1 : process et ressources, démarrer / arrêter / redémarrer, terminal web (prévus pour la v0.2) ;
  certificats Let's Encrypt (fournissez le vôtre) ; rotation du journal d'audit.

## Hôte

- Noyau 5.6 ou plus récent pour lire ; 5.8 ou plus récent pour éditer ou envoyer dans un conteneur actif. API Docker
  1.44 ou plus récente, stockage overlay2 ou containerd.
- Seules les dispositions de cgroups de Docker sont reconnues : avec le pilote cgroupfs et un `--cgroup-parent`
  personnalisé, un conteneur actif répond `container_changed` et ne s'explore pas. Avec le pilote systemd, cela
  marche.
- Mesuré sur Ubuntu (AppArmor) et Fedora (SELinux `enforcing`). Les autres modules de sécurité n'ont pas été mesurés.

## Fichiers

- Les pseudo-systèmes de fichiers (`/proc`, `/sys`…) apparaissent comme des dossiers qui ne s'ouvrent pas. Un type de
  pseudo-système que l'agent ne connaît pas ne serait pas refusé ; les nœuds de périphérique restent refusés quoi
  qu'il arrive.
- Un conteneur arrêté se lit par un index de son archive complète, construit au premier accès : cela peut prendre des
  secondes sur une grosse image, avec une barre de progression. L'index est plafonné à 4 Gio lus, 5 minutes et
  1 000 000 d'entrées (`too_large`) ; trois index sont gardés en mémoire.
- Un conteneur arrêté qui monte `/`, `/proc`, `/sys` (ou dessous) ou `/dev` de l'hôte ne s'explore pas (l'erreur
  nomme le montage) : son archive contiendrait l'hôte. Ses logs, ses changements et sa radiographie restent
  disponibles.
- Un dossier de l'hôte monté dans un conteneur arrêté, avec un montage `/proc`, `/sys`, FUSE ou NFS en dessous, n'est
  pas détecté : l'indexation peut alors faire lire ce montage à Docker, et un serveur FUSE ou NFS bloqué peut bloquer
  `dockerd`.
- Un montage FUSE d'un conteneur actif qui ne répond plus : les requêtes vers ce conteneur échouent après 10 secondes,
  4 au plus en même temps, et les lectures bloquées le restent dans l'agent jusqu'à son redémarrage.
- Les noms de fichiers qui ne sont pas en UTF-8 valide sont listés, mais ne s'ouvrent pas depuis l'interface.
- Le téléchargement d'un dossier d'un conteneur actif est refusé quand l'archive traverserait un montage de l'hôte,
  un pseudo-système ou un FUSE, et toujours pour `/`. Un montage ajouté entre le contrôle et le téléchargement (il
  faut `CAP_SYS_ADMIN` dans le conteneur) ou un montage NFS « dur » sous le dossier n'est pas refusé, et peut bloquer
  `dockerd`.
- Un montage ajouté plus tard sur l'hôte sous la source d'un montage de l'hôte n'apparaît pas dans le conteneur actif
  (propagation `rprivate` par défaut) et n'est pas détecté, alors que Docker peut le voir en construisant l'archive.
- Un conteneur ouvert arrêté, puis démarré avant le téléchargement : seul le contrôle des montages de l'hôte est
  refait, pas les autres contrôles de montage d'un conteneur actif.
- Un conteneur dont la racine du process a été déplacée par un chroot : l'explorateur montre la vue depuis la racine
  de son PID 1, et le téléchargement d'un dossier est refusé quand la table des montages n'a pas `/` ou un montage que
  Docker déclare. Une destination de montage visible telle quelle, sans lien symbolique sur son chemin, est prise sans
  preuve d'identité : un téléchargement peut alors bloquer `dockerd`.

## Écritures

- Pas atomiques : Docker supprime le fichier et en crée un nouveau. Les liens physiques vers l'ancien sont rompus, un
  process qui le garde ouvert voit encore l'ancien contenu, et une écriture coupée après une minute laisse le fichier
  perdu ou tronqué.
- Entre les contrôles et l'écriture, un process du conteneur peut encore modifier le fichier ou poser un lien
  symbolique dans le chemin ; Docker résout alors le chemin dans le conteneur. Il peut atteindre un autre montage du
  conteneur, comme le montage d'un dossier de l'hôte autre que `/`, `/proc`, `/sys` ou `/dev`.
- Refusées là où Docker n'écrit pas ce que voit le conteneur : `/etc/hosts`, `/etc/hostname`, `/etc/resolv.conf`,
  points de montage, tmpfs, `/dev`, `/proc`, `/sys`. Un tmpfs déclaré sous un autre tmpfs, un volume ou un montage
  fait refuser toute écriture dans ce conteneur actif (échec fermé).
- Montages que `docker inspect` ne montre pas (configs et secrets Swarm, `--init`, CDI ou `--gpus`) : une édition
  dans un conteneur actif est refusée, mais un envoi, ou toute écriture dans un conteneur arrêté, n'est pas contrôlé.
- Une écriture sous un montage FUSE bloqué ou NFS « dur » peut bloquer `dockerd`, et le conteneur reste verrouillé
  pour l'écriture jusqu'au redémarrage de l'agent ; l'interface peut alors dire à tort que l'écriture a peut-être eu
  lieu.
- Une prison chroot montée par le conteneur lui-même (`CAP_SYS_ADMIN`) n'est pas vue.
- Un tmpfs leurre monté par le conteneur lui-même (`CAP_SYS_ADMIN`) sur la cible d'un lien symbolique posé après le
  démarrage : une écriture peut alors réussir sans que le conteneur la voie (ce n'est pas une évasion).
- Conteneur arrêté dont la destination d'un tmpfs passe par un lien symbolique situé dans un volume ou un montage de
  l'hôte : un lien posé là plus tard (par l'hôte ou un autre conteneur) n'est pas vu par l'index, et le tmpfs peut
  être monté ailleurs au démarrage suivant ; une écriture peut alors réussir sans que le conteneur redémarré la voie
  (ce n'est pas une évasion).

## Logs

- Les logs s'affichent comme `docker logs` les montre : les secrets écrits par l'application ne sont pas masqués.
- Au-delà de 1 000 lignes par seconde, les lignes en trop ne s'affichent pas mais sont comptées ; le téléchargement
  les contient toutes.
- 8 flux au plus pour l'agent ; quand ils sont tous pris, le flux le plus ancien cède sa place et le dit.
- Un client parti sans fermer sa connexion (veille, changement de réseau) garde son flux ouvert côté agent pendant
  environ 15 minutes, et tient l'un des 8 créneaux jusque-là, sauf si un flux plus récent le lui prend.
- La reprise après une coupure peut rejouer quelques lignes de même horodatage, compter deux fois des lignes
  écartées, ou sauter des lignes après une rotation du journal ou un recul de l'horloge.
- Un pilote de logs que Docker ne sait pas relire (`none`, `syslog`…) donne `logs_unavailable`.

## Masquage des secrets

La radiographie masque les secrets par leur nom et par des formes connues. Non reconnus en v0.1, entre autres :

- les valeurs JSON (`{"token": "…"}`), `user:motdepasse@hôte` sans `://`, `password = …` avec des blancs, une
  variable nommée `DB_PWD` ;
- dans la commande : `mysql -pSECRET`, un secret dans un script ou dans un fichier passé en argument,
  `--password -s3cr3t`, `--pwd s3cr3t` en deux arguments, des clés avec des caractères non ASCII ou invisibles,
  `Authorization: Token …` ou `ApiKey …` ;
- certains masques laissent voir un reste de la valeur, et `env PWD=/srv` est masqué alors que ce n'est pas un
  secret.

La commande elle-même ne se révèle pas en v0.1 (`docker inspect` le peut).
