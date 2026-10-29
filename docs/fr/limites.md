# Limites connues

Cette page dit ce que Hublot-Kit ne fait pas, et les cas qu'il ne détecte pas, en connaissance de cause. Les specs
techniques donnent tous les détails : [v0.1 §13](../superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md), et
[v0.2 §9](../superpowers/specs/2026-10-23-hublot-kit-v0.2-design.md) pour ce qu'ajoute la v0.2.

## Périmètre

- Un serveur Docker, un utilisateur. Plusieurs serveurs ou utilisateurs : plus tard.
- Non pris en charge : Docker rootless, `userns-remap`, Podman, Kubernetes.
- Absents : process et ressources, démarrer / arrêter / redémarrer, terminal web (reportés après la v0.2) ;
  certificats Let's Encrypt (fournissez le vôtre) ; rotation du journal d'audit.

## Hôte

- Noyau 5.6 ou plus récent pour lire ; 5.8 ou plus récent pour éditer ou envoyer dans un conteneur actif. API Docker
  1.44 ou plus récente, stockage overlay2 ou containerd.
- Seules les dispositions de cgroups de Docker sont reconnues : avec le pilote cgroupfs et un `--cgroup-parent`
  personnalisé, un conteneur actif répond `container_changed` et ne s'explore pas. Avec le pilote systemd, cela
  marche.
- Mesuré sur Ubuntu (AppArmor) et Fedora (SELinux `enforcing`). Les autres modules de sécurité n'ont pas été mesurés.
- Connexions : 64 au plus en même temps par adresse (un /64 IPv6 compte pour une), 512 en tout. Qui dispose de
  plusieurs adresses (sur la machine elle-même, tout `127.0.0.0/8`) peut encore remplir les 512 et faire attendre les
  requêtes légitimes. Une page rebindée sur `127.0.0.1` (rebinding DNS, voir [sécurité](securite.md)) peut tenir les
  64 connexions de l'utilisateur local.

## Fichiers

- Les pseudo-systèmes de fichiers (`/proc`, `/sys`…) apparaissent comme des dossiers qui ne s'ouvrent pas. Un type de
  pseudo-système que l'agent ne connaît pas ne serait pas refusé ; les nœuds de périphérique restent refusés quoi
  qu'il arrive.
- Un conteneur arrêté se lit par un index de son archive complète, construit au premier accès : cela peut prendre des
  secondes sur une grosse image, avec une barre de progression. L'index est plafonné à 4 Gio lus, 5 minutes,
  1 000 000 d'entrées et 256 Mio de noms et de cibles de liens (`too_large`) ; trois index sont gardés en mémoire. Un
  index occupe environ 237 Mo par million d'entrées aux noms ordinaires ; un index mesuré juste sous le plafond de
  256 Mio de noms occupait 371 Mio, si bien que trois index peuvent dépasser 1 Gio.
- Pendant que Docker exporte un conteneur arrêté (indexation, ou téléchargement d'un dossier), il ne répond rien
  d'autre sur ce conteneur. Pendant l'indexation, la navigation répond tout de suite avec la progression ; les autres
  requêtes sur ce conteneur (onglets Aperçu et Secrets, logs, écritures, navigation pendant un téléchargement)
  attendent la fin de l'export, et échouent après 10 secondes (`docker_unavailable`) s'il dure plus.
- Un conteneur arrêté qui monte `/`, `/proc`, `/sys` (ou dessous) ou `/dev` de l'hôte ne s'explore pas (l'erreur
  nomme le montage) : son archive contiendrait l'hôte. Ses logs, ses changements et son détail (onglets Aperçu et
  Secrets) restent disponibles. Une source qui est un lien symbolique de l'hôte est jugée par sa cible, résolue
  depuis la racine de l'hôte (`/proc/1/root` : l'agent a besoin de `--pid=host`) ; une source que l'agent ne sait pas
  résoudre est refusée. Un lien changé sur l'hôte entre ce contrôle et le montage de la source par Docker n'est pas
  vu, ni un lien que seul `dockerd` voit (dossier partagé par Docker Desktop, `dockerd` dans son propre espace de
  noms de montage).
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
  écartées, ou sauter des lignes après une rotation du journal ou un recul de l'horloge (ou les marquer coupées, quand
  l'horloge recule de plus d'une seconde).
- Une ligne de plus de 16 Kio encore en cours d'écriture au moment de la coupure, et commencée plus d'une seconde
  avant elle, revient sans son début : la vue en direct perd ce début, mais marque la ligne comme coupée au lieu de
  la montrer complète. Le téléchargement contient la ligne entière.
- À l'ouverture de la vue, les lignes passées affichées peuvent commencer par la fin d'une ligne de plus de 16 Kio,
  sans marque : Docker compte les morceaux d'une ligne longue, pas les lignes, et ne permet pas de le détecter. Le
  téléchargement contient la ligne entière.
- Un pilote de logs que Docker ne sait pas relire (`none`, `syslog`…) donne `logs_unavailable`.

## Recherche (v0.2)

- La recherche de fichiers n'est possible que dans l'index déjà construit d'un conteneur arrêté que l'explorateur a ouvert. Pour un conteneur en marche, il n'y a pas d'index : ouvrez l'explorateur.
- La requête d'une recherche de fichiers part dans le corps d'un POST (`…/fs/search`), jamais dans une adresse : l'agent ne la journalise pas et ne la garde pas, et un journal d'accès ordinaire de reverse proxy ne la voit pas. Un proxy réglé pour journaliser les corps des requêtes la verrait : un nom de fichier peut être sensible.
- Pendant l'export d'un conteneur arrêté (construction de son index), Docker verrouille le conteneur. L'agent reconnaît cette construction par l'ID complet ou le nom du conteneur, jamais par un ID court : un appel direct à l'API avec un ID court attend alors le verrou, jusqu'au délai de la route (10 s pour les fichiers et la recherche ; les chiffres clés, eux, attendent ce verrou quel que soit l’ID, jusqu’à 30 s, v0.2 §5.3). L'interface envoie toujours l'ID complet.

## Images (v0.2)

- Les fichiers d'une image se lisent dans son export (`docker save`), sans créer de conteneur. Une couche compressée en **zstd** est refusée (`unsupported_image`) : seules les couches gzip et non compressées sont lues.
- Une image **multi-plateforme** dont plusieurs plateformes sont présentes en local est refusée (`unsupported_image`) : le manifeste de l'export a alors plusieurs entrées.
- **Coût d'une lecture** : l'aperçu d'un fichier relit l'export jusqu'à la fin de la couche qui le porte, couches précédentes comprises ; au pire, l'export entier (quelques secondes pour une image d'un gigaoctet). L'export occupe un cœur et le disque de Docker pendant sa durée, sans verrouiller de conteneur. Deux lectures au plus pour tout l'agent, une par image ; au-delà, l'aperçu attend et redemande.
- Plafonds de l'index : 4 Gio lus, 16 Gio décompressés, 1 000 000 d'entrées, 4 Kio par nom ou cible de lien, 256 Mio de noms, 5 minutes ; d'une lecture : 4 Gio, 16 Gio décompressés, 2 minutes. Au-delà, `too_large`.
- Pas de téléchargement des fichiers d'une image, et un aperçu de 2 Mio au plus.
- Historique de construction : les valeurs des arguments de construction sont masquées, quel que soit leur nom.
  Risque résiduel accepté : la valeur d'un `ARG` qui contient elle-même ` /bin/sh -c `, suivie d'une commande en forme
  exec ou d'un `SHELL` autre que sh ou bash, ne laisse qu'un marqueur de shell ; la suite de la ligne n'est alors
  masquée que par les règles ordinaires. L'historique ne permet pas de distinguer la valeur de la commande, et le cas
  est très peu probable.

## Snapshots (v0.2)

- Aucun contenu n'est gardé : deux photos se comparent par leurs métadonnées et leurs empreintes, sans diff ligne à ligne.
- La photo d'un conteneur en marche n'est pas atomique : un fichier modifié pendant le parcours peut être vu avant ou après sa modification. L'en-tête garde l'heure de début et celle de fin.
- La photo d'un conteneur arrêté passe par son export : Docker y met aussi ses volumes (lus, puis sautés, ils comptent dans les 4 Gio lus) et verrouille le conteneur jusqu'à 5 min.
- Plafonds : 500 000 entrées (au-delà, refus : un manifeste incomplet ferait croire à des suppressions) ; 1 Gio haché, puis les fichiers suivants sans empreinte (« partielle ») ; 256 Mio par fichier haché ; 10 000 entrées lues par dossier ; 10 min pour un conteneur en marche, 4 Gio et 5 min pour un export. Mémoire : un export garde toutes ses entrées jusqu'à la fin, environ 500 octets chacune (mesuré : un pic de 37 Mo pour 36 784 entrées), soit environ 250 Mo au plafond d'entrées ; les chemins et cibles de liens gardés sont plafonnés à 256 Mio en tout, comme l'index (au-delà, refus), soit environ 500 Mo au pire ; le parcours d'un conteneur en marche écrit au fil de l'eau (mesuré : 22 Mo). Le manifeste prend environ 42 octets par entrée une fois compressé (1,5 Mo pour 36 784).
- Rétention : 20 photos par conteneur et 1 Gio en tout, puis refus ; aucune suppression automatique.
- La clé perdue, supprimée ou changée (`rotate-snapshot-key`), les anciennes photos ne se comparent plus que par leurs métadonnées.
- Au-delà de 10 000 différences, la comparaison s'arrête : les fichiers de même taille et de même date qu'elle a comptés
  « inconnus » ne sont alors qu'un minimum, affiché « au moins N ».

## Masquage des secrets

L'agent masque les secrets par leur nom et par des formes connues. Non reconnus, entre autres :

- les valeurs JSON (`{"token": "…"}`), `user:motdepasse@hôte` sans `://`, `password = …` avec des blancs, une
  variable nommée `DB_PWD` ;
- dans la commande : `mysql -pSECRET`, un secret dans un script ou dans un fichier passé en argument,
  `--password -s3cr3t`, `--pwd s3cr3t` en deux arguments, des clés avec des caractères non ASCII ou invisibles,
  `Authorization: Token …` ou `ApiKey …` ;
- certains masques laissent voir un reste de la valeur, et `env PWD=/srv` est masqué alors que ce n'est pas un
  secret.

La commande elle-même ne se révèle pas (`docker inspect` le peut).
