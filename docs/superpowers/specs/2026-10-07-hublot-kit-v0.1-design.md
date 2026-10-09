# Hublot-Kit v0.1 — Spec technique

- **Date** : 2026-10-07
- **Statut** : validée le 2026-10-07
- **Cahier des charges (le quoi)** : https://claude.ai/code/artifact/86c33f3d-d4d1-4b9b-bae5-756b8276c139
- **Ce document (le comment)** : architecture, contrats et règles d'implémentation de la v0.1. En cas d'écart, le cahier des charges fait foi sur le périmètre, cette spec sur la technique.

## 1. Objectif

Un dev solo diagnostique un conteneur en panne sur son serveur Linux sans ouvrir de terminal : il ouvre un navigateur, choisit un conteneur, et voit ses fichiers, ce qui a changé depuis sa création, ses logs et sa config.

**Cible v0.1** : un serveur Docker, un utilisateur. Projet open source (MIT), dépôt `levy-cmd/hublot-kit`.

**Contexte projet** : 5 à 10 h par semaine ; l'agent est aussi le support d'apprentissage de Go. Les tâches du plan doivent donc rester petites et expliquées.

## 2. Périmètre

| # | Fonctionnalité | v0.1 |
|---|---|---|
| F1 | Liste des conteneurs (nom, image, statut, ports, uptime ; filtre, recherche) | oui |
| F2 | Explorateur : arborescence complète, aperçu texte et image, taille, droits | oui |
| F3 | Téléchargement d'un fichier ou d'un dossier (tar) | oui |
| F4 | Édition d'un fichier texte, envoi d'un fichier, avec confirmation | oui |
| F5 | Diff avec l'image (ajouté / modifié / supprimé depuis la création) | oui |
| F6 | Logs en direct : flux, recherche, pause, téléchargement | oui |
| F7 | Radiographie : env (secrets masqués), ports, réseaux, volumes, commande | oui |
| F8 | Process et ressources | v0.2 |
| F9 | Démarrer / arrêter / redémarrer | v0.2 |
| F10 | Terminal web | v0.2 |

Hors v0.1, en plus de F8 à F10 : HTTPS par Let's Encrypt (la v0.1 accepte un certificat fourni), rotation du journal d'audit, plusieurs serveurs, plusieurs utilisateurs, Docker rootless, userns-remap, Podman, Kubernetes.

## 3. Décisions

| Sujet | Décision | Raison |
|---|---|---|
| Agent | Go, dernière version stable | SDK Docker officiel, binaire statique, objectif d'apprentissage |
| Interface | SvelteKit + TypeScript, `adapter-static`, embarquée par `go:embed` | un seul binaire, rien d'autre à installer |
| Lecture des fichiers, conteneur actif | `/proc/<pid>/root` côté hôte + `filepath-securejoin/pathrs-lite` | rapide, marche sur distroless et scratch |
| Lecture des fichiers, conteneur arrêté | index des en-têtes tar de `GET /archive` sur `/`, en cache | le conteneur arrêté est le cas « pourquoi mon app plante » |
| Écriture | `PUT /containers/{id}/archive` uniquement, sur un chemin sans lien (§5.3) | marche sur conteneur arrêté ; Docker résout le chemin lui-même et sert de filet de sécurité ; notre contrôle « aucun lien dans la chaîne » garantit que le chemin écrit est exactement celui qui a été vérifié |
| Flux temps réel | SSE pour les logs ; WebSocket en v0.2 avec le terminal | un seul sens en v0.1, bibliothèque standard, reconnexion native |
| Routage HTTP | `net/http` (motifs Go 1.22+), pas de framework | moins de dépendances |
| Licence | MIT | adoption ; contributions réutilisables dans une future offre payante |
| i18n | dictionnaire maison FR / EN côté interface, l'API renvoie des codes | pas de bibliothèque pour deux langues |

**Dépendances Go autorisées** : le SDK Docker officiel, module `github.com/moby/moby/client` (suit Docker 29 ; tire `github.com/moby/moby/api` pour les types ; `github.com/docker/docker` s'arrête à v28.5.2) ; `github.com/cyphar/filepath-securejoin` v0.7.0, sous-paquet `pathrs-lite` (paquet `pathrs`, tire `cyphar.com/go-pathrs` en indirect) ; `golang.org/x/sys/unix`. Toute autre dépendance se justifie dans la PR.

**Dépendances interface** : SvelteKit, CodeMirror 6 (langages chargés à la demande : JSON, YAML, JavaScript, Python, shell, texte brut). Pas de bibliothèque de composants.

## 4. Architecture

Un agent unique tourne sur le serveur. Il sert l'interface, expose l'API, est le seul à parler au socket Docker et lit les fichiers par `/proc/<pid>/root`. Pas de base de données.

```
cmd/hublot/          point d'entrée : flags, config, démarrage, sous-commande reset-token
internal/
  dockerx/           interface + implémentation au-dessus du SDK Docker (version d'API négociée)
                     ListContainers, Inspect, Changes, Logs, GetArchive, PutArchive
  fsview/            lecture de l'arborescence d'un conteneur, une interface, deux implémentations
    fsview.go          type FS interface { List, Stat, Open, StatNoLinks, CheckArchive, Close } + types Entry
    procfs.go          conteneur actif
    tarindex.go        conteneur arrêté
  files/             seul point d'écriture : lecture seule, If-Match, audit, PutArchive
  secrets/           masquage des variables d'environnement
  auth/              jeton, sessions, limitation des tentatives
  audit/             journal JSONL en ajout seul
  api/               routes HTTP, SSE, en-têtes de sécurité, vérification d'Origin
  web/               go:embed du build SvelteKit
web/                 application SvelteKit
```

**Frontières**

- `api` ne parle qu'à `dockerx`, `fsview`, `files`, `secrets`, `auth`, `audit`. Jamais au SDK Docker directement.
- `fsview.FS` :

  ```go
  type Entry struct {
      Name       string
      Type       EntryType // file, dir, symlink, other
      Size       int64
      Mode       fs.FileMode // setuid/setgid/sticky convertis, voir plus bas
      UID, GID   int
      ModTime    time.Time
      LinkTarget string    // brut, jamais résolu pendant le listing
  }

  type FS interface {
      List(ctx context.Context, dir string) (entries []Entry, truncated bool, err error)
      Stat(ctx context.Context, path string) (Entry, error)
      Open(ctx context.Context, path string) (io.ReadCloser, error)
      // chemin nettoyé, aucun lien sur toute la chaîne (§5.3 étape 2, §5.4)
      StatNoLinks(ctx context.Context, path string) (Entry, error)
      // avant tout GetArchive d'un dossier : ni bind de l'hôte (§5.2 étape 7),
      // ni pseudo-système ou FUSE sur, sous ou au-dessus du dossier (§5.4)
      CheckArchive(ctx context.Context, dir string) error
      Close() error // procfs : ferme les descripteurs de la racine et du pidfd
  }
  ```

  `Mode` : les bits de permission de `st_mode` (ou du mode tar) passent tels quels, mais setuid, setgid et sticky n'ont pas les mêmes valeurs dans `st_mode` (`S_ISUID`, `S_ISGID`, `S_ISVTX`) et dans `fs.FileMode` : la conversion les mappe explicitement sur `fs.ModeSetuid`, `fs.ModeSetgid`, `fs.ModeSticky`, avec un test par bit.

  Un constructeur `fsview.For(ctx, docker, containerID)` renvoie `procfs` si le conteneur tourne, `tarindex` sinon. Le reste du code ignore d'où viennent les fichiers.
- `dockerx.Client` est une interface : les tests unitaires utilisent un faux, les tests d'intégration un vrai Docker.

## 5. Accès aux fichiers

### 5.1 Conteneur actif (`procfs`)

Comparer deux `Inspect` ne suffit pas à prouver que la racine ouverte est celle du conteneur : un PID peut être réattribué avant que Docker ne voie l'arrêt. On accroche donc l'identité du process avec un **pidfd**, qui désigne un process précis et ne peut pas être réattribué :

1. `Inspect` → `State.Pid`. `Pid == 0` → bascule sur `tarindex`.
2. `pidfd_open(pid)` → `pidfd`. Échec (process disparu) → `container_changed`.
3. Lire `/proc/<pid>/cgroup` et découper le chemin en composants. **Ancrage sur le composant le plus externe qui a la forme d'un ID Docker** : en partant de la racine, le premier composant `docker-<64 hex>.scope` (pilote systemd), ou `<64 hex>` placé directement sous un composant `docker` (pilote cgroupfs). L'ID de ce composant doit être l'ID complet du conteneur (64 caractères, composant entier, jamais une sous-chaîne), sinon `container_changed`. Aucun composant de cette forme → `container_changed`.
   - Les cgroups **enfants** de ce composant sont acceptés : docker:dind en cgroup v2 déplace son PID 1 dans `/system.slice/docker-<id>.scope/init`.
   - `docker-A.scope/docker-<B>.scope` est refusé pour B : A, plus externe, est trouvé en premier et ne correspond pas. Un conteneur à qui on a délégué ses cgroups ne peut donc pas passer pour un autre (le contrôle `pidfd` de l'étape 5 reste requis).
   - Tout chemin qui contient un composant `..` est refusé, quelle que soit la forme (`/../<id>`, `/../docker-<id>.scope`…) : l'agent tourne toujours avec `--cgroupns=host` (§8) ou sur l'hôte.
   - **Limite v0.1** : pilote cgroupfs avec un `--cgroup-parent` personnalisé (chemin `/<parent>/<id>`, sans composant `docker`) → refusé, `container_changed`. Avec le pilote systemd, un `--cgroup-parent` garde la forme `docker-<id>.scope` et reste accepté.

   Ce contrôle prouve que le process épinglé par `pidfd` appartient au conteneur.
4. Ouvrir `/proc/<pid>/root` en `O_PATH | O_DIRECTORY` → `rootfd`.
5. `pidfd_send_signal(pidfd, 0)` doit réussir : le process est toujours vivant, donc le PID n'a pas été réattribué entre les étapes 2 et 4, et `rootfd` est bien sa racine. Sinon → fermer, `container_changed`.
6. Ouvrir le chemin demandé par `pathrs.OpenatInRoot(rootfd, chemin)` (`github.com/cyphar/filepath-securejoin/pathrs-lite` v0.7.0, paquet `pathrs` ; le paquet racine n'exporte plus ces fonctions), qui renvoie un descripteur `O_PATH`. Puis, dans cet ordre :
   - **Pseudo-système** : `fstatfs` du descripteur `O_PATH`, **avant tout `Reopen`**, pour un dossier comme pour un fichier, et aussi pour `Stat` → `forbidden_path` (étape 9).
   - **Dossier** : `pathrs.Reopen(handle, O_RDONLY|O_DIRECTORY)`, pour lister uniquement.
   - **Fichier** (`Open`, aperçu, téléchargement) : `fstat` du descripteur `O_PATH` ; tout ce qui n'est pas `S_IFREG` (périphérique, FIFO, socket) est refusé **avant** `Reopen` (`not_regular`). Puis `pathrs.Reopen(handle, O_RDONLY|O_NONBLOCK)`. Raison : root dans un conteneur peut créer par `mknod` un nœud de périphérique de l'hôte, et l'agent est hors du cgroup devices du conteneur : l'ouvrir lirait le périphérique de l'hôte.

   **Interdits** : `filepath.Join`, `os.Open` sur un chemin construit à la main, tout suivi de lien hors `pathrs`.
7. `List` : sur le dossier rouvert, lire les noms, puis pour chaque nom `fstatat(dirfd, nom, AT_SYMLINK_NOFOLLOW)` et, pour un lien, `readlinkat(dirfd, nom)`. Tout se fait relativement au descripteur du dossier, avec un seul composant de chemin : aucun lien n'est suivi et rien ne peut sortir de la racine. Au-delà de 10 000 entrées : tronquer, `truncated = true`.
8. Navigation sur un lien : l'interface demande le chemin cible, résolu par `pathrs.OpenatInRoot` comme n'importe quel autre chemin.
9. **Pseudo-systèmes de fichiers non parcourables** (`/proc`, `/sys` et assimilés) : le listing de `/` les montre comme des dossiers ordinaires (aucun champ `Entry` en plus : le listing fait `fstatat` sur le nom, sans `fstatfs`) ; les ouvrir renvoie `forbidden_path`, que l'interface affiche comme un dossier inaccessible. Raison : leurs pseudo-fichiers sont lus avec les privilèges hôte de l'agent (ex. `/proc/kallsyms` donne les vraies adresses du noyau). Le contrôle porte sur le **descripteur résolu**, pas sur le texte du chemin, donc un lien qui y mène est refusé aussi : `fstatfs` du descripteur `O_PATH` (étape 6) ; `f_type` parmi les 15 types ci-dessous → `forbidden_path`. S'applique à `List`, `Stat` et à la lecture d'un fichier.
   - Dans `golang.org/x/sys/unix` (v0.48.0) : `PROC_SUPER_MAGIC`, `SYSFS_MAGIC`, `CGROUP_SUPER_MAGIC`, `CGROUP2_SUPER_MAGIC`, `DEBUGFS_MAGIC`, `TRACEFS_MAGIC`, `SECURITYFS_MAGIC`, `BPF_FS_MAGIC`, `PSTOREFS_MAGIC`, `EFIVARFS_MAGIC`, `BINFMTFS_MAGIC`, `SELINUX_MAGIC` (`0xf97cff8c`), `SMACK_MAGIC` (`0x43415d53`).
   - Constantes locales, absentes de x/sys : `CONFIGFS_MAGIC = 0x62656570` (`include/uapi/linux/magic.h` du noyau actuel ; absente des en-têtes 6.12) et `FUSE_CTL_SUPER_MAGIC = 0x65735543` (`fs/fuse/control.c`, aucun en-tête uapi).

   `/dev` reste parcourable : le lister est sans danger, et ses nœuds de périphérique sont déjà refusés par la règle des fichiers réguliers (étape 6, `not_regular`).

Spike : avec `--cgroupns=host`, l'agent conteneurisé lit `0::/docker/<id>` (Docker Desktop, pilote cgroupfs) ou `0::/system.slice/docker-<id>.scope` (Ubuntu, pilote systemd) ; le binaire sur l'hôte marche aussi. Sans `--cgroupns=host`, Docker Desktop donne `0::/../<id>` et Ubuntu `0::/../docker-<id>.scope` : le spike refusait le premier et acceptait encore le second ; le socle refuse les deux (composant `..`). 1 000 entrées listées en 1 à 15 ms (Docker Desktop ; Ubuntu : 3 × 1 000 entrées OK, durées non extraites). Noyau requis pour `pidfd_open` : 5.3, déjà couvert par le 5.6+ exigé pour `openat2`. Cible `--privileged` ou `apparmor=unconfined` : l'ouverture de `/proc/<pid>/root` exige un agent lui-même `apparmor=unconfined` sur un hôte AppArmor (spike n°3b, §8). Fedora (SELinux) : même chemin cgroup qu'Ubuntu, l'agent `--pid=host` tourne en `spc_t` et lit toutes les cibles mesurées sans flag de plus.

### 5.2 Conteneur arrêté (`tarindex`)

1. Clé de cache : `containerID + State.FinishedAt`. Un redémarrage puis un arrêt changent la clé. Une écriture réussie sur le conteneur arrêté (§5.3) supprime son index du cache ; il est reconstruit au prochain accès.
2. Construction en un seul exemplaire par clé (verrou de type singleflight). Pendant la construction, `List`, `Stat`, `Open` et le téléchargement d'un dossier renvoient `ErrIndexing` avec la progression (`indexing`, 202).
3. `GetArchive("/")` lu en flux par `archive/tar` ; seuls les en-têtes sont gardés, le contenu est lu et jeté. Progression = octets lus / `SizeRootFs` (`Inspect` avec `size=true`), **plafonnée à 99 %** jusqu'à la fin du tar : les volumes et binds sont dans l'archive mais pas dans `SizeRootFs`, le rapport peut dépasser 100 %. Construction plafonnée en octets lus **et** en durée (valeurs proposées : 4 Gio, 5 minutes) : au-delà, elle est abandonnée → `too_large`.
4. Index en mémoire : `map[dir][]Entry`. Au plus 3 index en cache (LRU), pour l'objectif de 50 Mo au repos hors index actif.
5. **Résolution** : tout chemin demandé est résolu composant par composant dans l'index. Les composants `.` et vides sont ignorés ; une barre oblique finale exige que le résultat soit un dossier. Chaque composant est cherché ; un lien rencontré est résolu avant de continuer : cible relative depuis le dossier parent du lien, cible absolue depuis la racine du conteneur, `..` bloqué à la racine, puis le parcours reprend. Un `..` qui suit un dossier atteint par un lien remonte au parent de la **cible résolue**, pas au parent du lien (même règle que le noyau). **Un seul compteur de 40 sauts** pour toute la résolution, cibles imbriquées comprises : une cible qui passe elle-même par des dossiers liés ne le remet pas à zéro. Seul le chemin entièrement résolu, sans lien, est envoyé à `GET /archive` ; jamais un `linkname` tel quel. Raison : `GET /archive` ne suit pas un lien en dernier composant (il renvoie l'entrée du lien : typeflag `'2'`, `linkname` brut, taille 0). `HEAD /archive` n'est plus utilisé. `Stat`, `Open` et le téléchargement d'un dossier attendent donc l'index, comme `List`.
6. `Stat(path)` : entrée de l'index au bout de la résolution ; `Stat` suit donc les liens comme tout chemin résolu (c'est le listing qui montre les liens eux-mêmes, avec `LinkTarget`). `Open(path)` : `GetArchive(chemin résolu)` et lecture de l'unique entrée ; entrée caractère, bloc ou FIFO → `not_regular`. `Entry.LinkTarget` est toujours la cible brute : `linkname` de l'en-tête tar, jamais une cible résolue par le démon. **Liens physiques** (typeflag `'1'`, taille 0, `linkname` = chemin de la première occurrence dans l'archive) : entrée de type fichier, taille et mode repris de l'entrée cible ; `Open` résout par l'index et lit l'entrée cible.
7. **Binds de l'hôte** : avant toute construction, lire `Inspect.Mounts`. Si un montage de type `bind` a pour `Source` `/`, ou `/proc`, `/sys` ou un chemin en dessous, ou exactement `/dev`, l'explorateur du conteneur arrêté est **indisponible pour ce conteneur** : `List`, `Stat`, `Open` et le téléchargement → `forbidden_path`, avec un détail qui nomme le bind (ex. `bind /proc -> /host/proc`) ; aucun index n'est construit. Un bind de `/dev` entier reste bloqué : tout l'arbre de périphériques de l'hôte, contenu de `/dev/shm` compris, deviendrait archivable et indexable. Les binds d'un seul périphérique ou socket (ex. `/dev/log`, `/dev/null`) sont sans danger : l'archive n'en contient que l'en-tête tar et `Open` les refuse (`not_regular`). Un volume nommé du pilote `local` dont les options `o` (séparées par des virgules, ex. `o=bind,ro`) contiennent `bind` ou `rbind`, avec `device=…`, est traité comme un bind (sa `Source` réelle est la valeur de `device`) : l'agent lit `Inspect.Mounts[].Driver` et, pour `local`, les options du volume via l'API d'inspection de volume (`GET /volumes/{nom}`, champ `Options`). Les binds de dossiers contenant un sous-montage `/proc` ou `/sys` ne sont pas détectés (§13). Les logs, le diff et la radiographie restent disponibles. Raison : Docker construit `GET /archive` de `/` avec tous les points de montage du conteneur montés, binds compris ; pour un conteneur de supervision (node-exporter, cAdvisor, Netdata, Promtail) qui monte `/proc`, `/sys` ou `/` de l'hôte, l'archive contiendrait l'hôte : `Open` servirait des pseudo-fichiers de l'hôte, et dockerd pourrait lire `/proc/kcore` sans jamais finir.
   Sans ces binds, les entrées sous `/proc` et `/sys` sont ignorées : les pseudo-systèmes propres du conteneur ne sont pas montés pour l'archive d'un conteneur arrêté (seuls ses volumes et binds le sont), ce qui suffit à la place du contrôle `fstatfs` de §5.1. `/dev` est montré : ses en-têtes sont sans danger, et `Open` refuse les périphériques.

Spike : `GET /archive` de `/` sur un conteneur arrêté **inclut le contenu des volumes nommés** (mesuré : Docker Desktop, Ubuntu containerd, Ubuntu overlay2) ; l'index les montre comme le reste. Les binds y sont aussi : Docker monte tous les points de montage du conteneur, volumes et binds, pour construire l'archive (code de Docker, non mesuré par le spike ; d'où l'étape 7). `HEAD` et `GET /archive` répondent sur un conteneur arrêté ; seul `GET` est gardé. Mesuré sur alpine : 523 entrées, ~8,9 Mo lus, 217 à 276 ms.

### 5.3 Écriture (`files`)

Ordre strict, dans une seule fonction :

1. Mode lecture seule → `read_only`.
2. Contrôler la cible. **Chemin d'écriture** = le chemin de la requête nettoyé (`path.Clean`, absolu), et rien d'autre : on ne résout pas les liens, on les refuse. Ce chemin doit être **sans lien** sur toute la chaîne, dossier parent et dernier composant compris :
   - conteneur actif : ouverture par `openat2` sur `rootfd` (§5.1 étapes 1 à 5) avec `RESOLVE_IN_ROOT | RESOLVE_NO_SYMLINKS | RESOLVE_NO_MAGICLINKS`, en `O_PATH` ; `ELOOP` → un lien est présent. Aucun lien n'est suivi, ce qui respecte l'interdit de §5.1 étape 6 ;
   - conteneur arrêté : l'index (§5.2) montre qu'aucun composant de la chaîne n'est un lien ; `indexing` tant qu'il se construit, `forbidden_path` si l'explorateur est indisponible (§5.2 étape 7).

   Équivalent pour un conteneur arrêté des contrôles ci-dessous : le type de l'entrée d'index remplace `fstat` (non régulier → `not_regular`) ; il n'y a pas de `fstatfs`, la règle des binds de §5.2 étape 7 s'applique à sa place.

   Un lien n'importe où dans la chaîne → `link_in_path`. Puis : le descripteur obtenu (la cible pour une édition, le dossier parent pour un envoi) ne doit pas être dans un pseudo-système de fichiers (`fstatfs`, §5.1 étape 9), sinon `forbidden_path` ; édition, la cible doit être un fichier régulier, sinon `not_regular`. Le dernier composant d'un envoi est contrôlé à l'étape 3. Le nom du fichier envoyé doit être un composant unique : ni `/`, ni `.`, ni `..`, ni vide, sinon `400 invalid_name`.
3. Relire le fichier par `fsview`, calculer son SHA-256, comparer à `If-Match` → `conflict` si différent. Pour un nouveau fichier (envoi), `If-None-Match: *` : le test d'existence porte sur le dernier composant **sans suivre de lien** (sémantique `lstat` : `fstatat(parentfd, nom, AT_SYMLINK_NOFOLLOW)`, ou l'index) ; absent → on continue ; lien, même pendant → `link_in_path` ; toute autre entrée → `conflict`.
4. `audit` : écrire l'intention.
5. Construire un tar d'une entrée qui reprend `Mode`, `UID`, `GID` du fichier existant (nouveau fichier : `0644`, uid/gid du dossier parent), puis `PutArchive` sur le dossier parent **du chemin d'écriture validé à l'étape 2** : c'est exactement cette chaîne qui part vers Docker, jamais un chemin résolu ni recalculé.
6. `audit` : écrire le résultat. Conteneur arrêté : en cas de succès, son index est retiré du cache (§5.2).

Limite assumée : entre l'étape 2 et l'étape 5, un process du conteneur peut encore modifier le fichier, ou remplacer un composant par un lien après les contrôles de l'étape 2. Docker résout alors le chemin lui-même, dans la racine du conteneur : c'est le filet de sécurité. Notre contrôle garantit la cohérence (on écrit là où on a vérifié), pas l'atomicité. Pour un seul utilisateur, c'est acceptable.

### 5.4 Téléchargement

- Fichier : `fsview.Open` (contrôles `not_regular` et `forbidden_path`), relayé en flux.
- Dossier : même règle que l'écriture (§5.3 étape 2) : le chemin de la requête nettoyé, sans lien sur toute la chaîne, sinon `link_in_path`.
  - Conteneur actif : ouverture par `openat2` `RESOLVE_IN_ROOT | RESOLVE_NO_SYMLINKS | RESOLVE_NO_MAGICLINKS` sur `rootfd`, puis `fstatfs` du descripteur (`forbidden_path`, §5.1 étape 9). Vient ensuite le **contrôle des montages** (`FS.CheckArchive`) : Docker construit le tar avec les binds et volumes du conteneur montés dedans, et un bind du `/proc` de l'hôte y met `/proc/kcore` (254 Tio, dockerd bloqué : mesuré en revue). Deux contrôles :
    - (a) la règle des binds de §5.2 étape 7 sur `Inspect.Mounts`, volumes `local` en `o=bind`/`rbind` compris, source non absolue refusée → `forbidden_path` qui nomme le montage. Pour ce tar, sont refusés aussi les types de montage autres que bind, volume, tmpfs et image, les volumes `local` dont `type=` est un pseudo-système ou FUSE, les volumes d'un pilote tiers et ceux dont les options ne se lisent pas (ce qu'ils montent est inconnu). Ces extensions ne valent pas, pour l'instant, pour l'explorateur d'un conteneur arrêté (§5.2) ;
    - (b) `/proc/<pid>/mountinfo`, lu pour le process épinglé par le pidfd, qui doit être vivant après la lecture. On y cherche les montages de l'un des 15 types de §5.1 étape 9 ou FUSE (`fuse`, `fuseblk`, `fuse.*`, `fuseblk.*`). La table des montages doit montrer la racine `/` et chaque destination d'`Inspect.Mounts` telle quelle, sinon refus (PID 1 chrooté, destination sous un lien). Un tel montage sur ou sous la destination d'un montage d'`Inspect.Mounts` refuse **tout** tar du conteneur, car dockerd remonte ces montages pour chaque tar. Un tel montage sur, sous ou au-dessus du dossier refuse ce tar. Dans les deux cas → `forbidden_path`. Une ligne illisible fait refuser.

    Le `/` d'un conteneur actif est donc refusé, car son propre `/proc` est dessous. Puis `GetArchive` sur exactement le chemin nettoyé, relayé en `.tar` sans mise en mémoire ; le créneau de la garde (§13) reste pris jusqu'à la fin de la copie.

    Limite assumée : entre le contrôle et `GetArchive`, un montage peut apparaître dans le conteneur. Monter quoi que ce soit dans un conteneur actif exige `CAP_SYS_ADMIN` dans le conteneur (conteneur privilégié, ou `--cap-add SYS_ADMIN` : par exemple rclone ou s3fs avec `/dev/fuse`), ou root sur l'hôte (propagation `rshared`) ; limite assumée pour la v0.1. dockerd construit le tar à partir des montages configurés : mesuré sur Docker Desktop, il ne voit pas le `/proc` propre du conteneur. Et la règle « tout le conteneur » couvre ce qui est monté sous un montage Docker au moment du contrôle. À remesurer sur une VM Ubuntu.
  - Conteneur arrêté : l'index montre qu'aucun composant n'est un lien (§5.2 ; `indexing` pendant la construction, `forbidden_path` si l'explorateur est indisponible), puis `GetArchive(chemin nettoyé)` relayé de même.
  - Pour télécharger un dossier atteint par un lien, l'interface navigue d'abord vers la cible (§5.1 étape 8).
- `&check=1` : tous les contrôles, puis `204` sans rien lire (aucun `GetArchive`). L'interface vérifie ainsi avant de laisser le navigateur télécharger. Il vaut pour un fichier comme pour un dossier ; une requête `HEAD` est traitée de même.
- Toujours `Content-Disposition: attachment`.

## 6. API

Préfixe `/api`, JSON. Erreur : `{"error": "<code>", "detail": "<texte libre pour les logs>"}`. Codes : `unauthorized`, `invalid_name` (nom d'envoi qui n'est pas un composant unique, 400), `forbidden_origin`, `rate_limited`, `not_found`, `read_only`, `conflict`, `container_changed`, `indexing`, `too_large`, `not_regular` (pas un fichier régulier : périphérique, FIFO, socket), `forbidden_path` (le chemin résolu tombe dans un pseudo-système de fichiers, §5.1 étape 9, ou conteneur arrêté dont un bind expose l'hôte, §5.2 étape 7), `link_in_path` (écriture ou téléchargement de dossier : un lien dans la chaîne du chemin, §5.3 étape 2), `docker_unavailable`, `internal`. Conteneur arrêté : toute route de fichiers peut aussi renvoyer `too_large` (construction de l'index plafonnée, §5.2 étape 3). Statuts HTTP : `not_found` 404 (`ELOOP` compris : boucle de liens, lien magique refusé), `forbidden_path` 403, `container_changed` 409, `not_regular` et `link_in_path` 422, `too_large` 413, `docker_unavailable` 503, `internal` 500. Conteneur arrêté, en attendant l'index (Explorateur 2) : 501 `internal`, temporaire (Q4). Garde des routes de fichiers (§13) : 503 `internal` si 4 opérations sont déjà en cours sur le conteneur, 504 `internal` après 10 s ; un créneau n'est rendu qu'au vrai retour de l'opération, copie d'un téléchargement comprise.

| Méthode | Route | Rôle |
|---|---|---|
| POST | `/api/login` | `{token}` → cookie de session |
| POST | `/api/logout` | |
| GET | `/api/meta` | `{version, readOnly}` |
| GET | `/api/containers` | F1 |
| GET | `/api/containers/{id}` | F7, env masquées par l'agent |
| POST | `/api/containers/{id}/env/reveal` | `{name}` → valeur en clair, journalisé |
| GET | `/api/containers/{id}/fs?path=` | F2 liste ; `202 {"error":"indexing","progress":0.42}` pendant l'indexation ; `forbidden_path` |
| GET | `/api/containers/{id}/fs/file?path=` | F2 aperçu : `text/plain` ou image, plafond 2 Mo, sinon `too_large` ; `not_regular`, `forbidden_path`, `indexing` ; en-tête `ETag` = SHA-256 |
| GET | `/api/containers/{id}/fs/download?path=` | F3 ; `indexing` (conteneur arrêté) ; fichier : `not_regular`, `forbidden_path` ; dossier : `link_in_path`, `forbidden_path` ; `&check=1` : contrôles seuls, `204` ; dossier d'un conteneur actif : `forbidden_path` si le tar traverserait un bind de l'hôte, un pseudo-système ou un FUSE (§5.4) |
| PUT | `/api/containers/{id}/fs/file?path=` | F4 édition, `If-Match` obligatoire ; `link_in_path` (lien dans la chaîne, dernier composant compris), `not_regular`, `forbidden_path`, `indexing` |
| POST | `/api/containers/{id}/fs/upload?dir=` | F4 envoi, multipart, `If-None-Match: *` ; `link_in_path` (lien dans la chaîne, ou nom existant qui est un lien, même pendant), `conflict`, `forbidden_path` (dossier dans un pseudo-système), `invalid_name`, `indexing` |
| GET | `/api/containers/{id}/changes` | F5 `[{path, kind: "added"|"modified"|"deleted"}]` |
| GET | `/api/containers/{id}/logs?tail=500` | F6 SSE : `{ts, stream: "stdout"|"stderr", line}` |
| GET | `/api/containers/{id}/logs/download` | F6 fichier texte |

Tout sauf `/api/login` et les fichiers statiques exige une session.

**Aperçu** : un octet nul dans les 8 premiers Ko → binaire, pas d'aperçu texte. Images : PNG, JPEG, GIF, WebP détectées par `http.DetectContentType`. **SVG et HTML sont toujours servis comme texte.**

## 7. Sécurité

L'agent a, de fait, les droits root sur l'hôte. Le README le dit en tête. Les mesures ci-dessous limitent les erreurs et l'exposition ; elles ne protègent pas contre un agent compromis.

- **Écoute** : `127.0.0.1:7878` par défaut. Adresse non locale sans TLS → avertissement visible au démarrage (cas légitime : dans un conteneur, ou derrière un reverse proxy).
- **Jeton** : 32 octets de `crypto/rand`, encodés en base64url, affichés une seule fois au premier lancement. Stocké sous forme d'empreinte SHA-256 dans `<data-dir>/token.sha256`. `hublot reset-token` en génère un nouveau. Comparaison par `subtle.ConstantTimeCompare`.
- **Tentatives** : 5 échecs par minute et par IP → `rate_limited` (429). Chaque échec est journalisé, et les tentatives bloquées aussi (résultat `rate_limited`, au plus une ligne `rate_limited` par IP et par minute).
- **Session** : 32 octets aléatoires, en mémoire, 12 h. Cookie `HttpOnly`, `SameSite=Strict`, `Secure` si TLS ou si `--public-origin` est en https, `Path=/`.
- **CSRF** : sur toute méthode autre que GET/HEAD, l'en-tête `Origin` doit être présent, bien formé, différent de `null`, et égal à l'**origine de confiance** comparée en entier (schéma, hôte, port), sinon `forbidden_origin`. Origine de confiance : `--public-origin` si défini (obligatoire derrière un reverse proxy, qui change souvent le schéma), sinon `http(s)://` + l'en-tête `Host` de la requête, le schéma venant de la présence de TLS sur l'agent. Les en-têtes `X-Forwarded-*` ne sont jamais crus. Les GET, flux SSE compris, ne modifient rien : le cookie `SameSite=Strict` suffit (les navigateurs n'envoient pas toujours `Origin` sur un GET de même origine, l'exiger casserait le flux). Rebinding DNS : sans `--public-origin`, l'origine de confiance dérive de Host, donc un nom rebindé passe le contrôle d'Origin ; c'est le cookie de session `SameSite=Strict` lié au vrai hôte et le jeton qui protègent alors. Exposé hors de 127.0.0.1, renseignez `--public-origin`. Une page rebindée peut aussi épuiser la limite de tentatives de 127.0.0.1 et bloquer la connexion locale pendant une minute.
- **En-têtes** : `Content-Security-Policy: default-src 'self'; script-src 'self' 'sha256-…'; style-src 'self' 'unsafe-inline'; img-src 'self' blob: data:; frame-ancestors 'none'`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`. L'`index.html` produit par SvelteKit (adapter-static) démarre l'application par un script en ligne qui embarque la version du build : au démarrage, l'agent lit l'`index.html` embarqué et ajoute à `script-src` l'empreinte SHA-256 de chaque script en ligne (sans `'unsafe-inline'` pour les scripts). Interface non compilée : `script-src 'self'` seul. `style-src 'unsafe-inline'` est accepté, car SvelteKit pose un style en ligne sur `#svelte-announcer` ; le risque reste faible puisque `{@html}` est interdit (aucune injection de balisage) et que les scripts restent verrouillés.
- **XSS par contenu de conteneur** : l'agent ne sert jamais un fichier de conteneur avec un type exécutable par le navigateur (HTML, SVG, XML, JavaScript). Règles :
  - L'aperçu ne renvoie que `text/plain; charset=utf-8` ou l'un des quatre types d'image matricielle de §6, toujours avec `nosniff`. Un SVG est un document actif : il n'est **jamais** rendu comme image, ni dans `<img>`, ni dans `<object>`, ni dans `<iframe>`. Il s'affiche comme texte.
  - Côté interface, le contenu d'un conteneur (fichiers, logs, noms de fichiers, variables d'env) passe toujours par l'échappement de Svelte. `{@html}` est interdit dans tout le code, et une règle ESLint le fait respecter.
  - Le téléchargement force `Content-Disposition: attachment` et `application/octet-stream`.
- **Secrets** : une variable est masquée si son nom contient (sans tenir compte de la casse) `PASSWORD`, `PASSWD`, `PASS`, `TOKEN`, `KEY`, `SECRET`, `CREDENTIAL`, `PRIVATE`, `DSN`, ou si sa valeur contient une URL avec identifiants (`://user:pass@`). Valeur masquée renvoyée : `null` avec `masked: true`.
- **Lecture seule** : `--read-only` ; appliqué dans `files` et nulle part ailleurs (un seul point d'écriture).
- **Binds de l'hôte (conteneur actif)** : un conteneur lancé avec `-v /:/host` montre l'hôte dans l'explorateur ; par exemple, `/host/etc/shadow` est lisible. La lecture et le listing ne sont pas refusés : l'utilisateur de l'interface est de fait root sur l'hôte (début de §7), et les pseudo-systèmes et les périphériques restent refusés. Le tar d'un dossier, lui, est refusé pour tout le conteneur dès qu'un montage le rend dangereux, et le refus nomme le montage : bind sensible (`/`, `/proc`, `/sys` ou dessous, `/dev` : §5.2 étape 7), volume `type=` pseudo/FUSE, volume d'un pilote tiers, volume aux options illisibles (§5.4).
- **Journal d'audit** : `<data-dir>/audit.log`, une ligne JSON par événement : `{ts, action, container, path, result, error, remote}`. Actions : `login.fail`, `env.reveal`, `file.write`, `file.upload`. `remote` : IP de la connexion TCP (jamais `X-Forwarded-For`), renseignée pour `login.fail`. Écrit avant et après chaque écriture.
- **Socket** : la documentation ne dit jamais que `:ro` protège quoi que ce soit. Un proxy de socket est mentionné comme complément optionnel.

## 8. Configuration et installation

Flags (chacun a son équivalent en variable `HUBLOT_*`) :

| Flag | Défaut |
|---|---|
| `--listen` | `127.0.0.1:7878` (`0.0.0.0:7878` dans l'image Docker) |
| `--data-dir` | `/var/lib/hublot-kit` (`/data` dans l'image) |
| `--read-only` | `false` |
| `--tls-cert`, `--tls-key` | vides |
| `--public-origin` | vide (ex. `https://hublot.mondomaine.fr` derrière un reverse proxy) |

**Installation recommandée**. Minimum mesuré par le spike sur Ubuntu (AppArmor actif) pour des cibles sous le profil `docker-default`, et identique sur Fedora (SELinux `enforcing`) : `--pid=host --cap-add SYS_PTRACE` + socket. `--cgroupns=host` est requis par le socle partout où l'agent tourne en conteneur, car tout `..` est refusé (§5.1 étape 3) ; il n'est inutile que pour le binaire lancé sur l'hôte. `--security-opt apparmor=unconfined` est requis pour explorer les cibles `--privileged` ou `apparmor=unconfined` sur un hôte AppArmor (spike n°3b) ; il est sans effet sur un hôte sans AppArmor. `--security-opt label=disable` est sans effet sur un hôte sans SELinux. La même commande sert aux deux familles (mesuré sur Ubuntu et Fedora) :

```sh
docker run -d --name hublot-kit \
  --pid=host --cgroupns=host --cap-add SYS_PTRACE \
  --security-opt apparmor=unconfined \
  --security-opt label=disable \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v hublot-data:/data \
  -p 127.0.0.1:7878:7878 \
  ghcr.io/levy-cmd/hublot-kit:0.1
```

| Droit | Raison |
|---|---|
| `--pid=host` | voir `/proc/<pid>` des conteneurs |
| `--cgroupns=host` | requis partout : avec un espace de noms cgroup privé, l'agent lit `/../<id>` (pilote cgroupfs, Docker Desktop) ou `/../docker-<id>.scope` (pilote systemd, Ubuntu), et le socle refuse tout composant `..` (§5.1 étape 3). Le spike passait sans ce flag sur Ubuntu parce qu'il acceptait encore `..` |
| `--cap-add SYS_PTRACE` | obligatoire pour une cible qui tourne sous un autre uid (mesuré : sans lui, `/proc/<pid>/root` renvoie `EACCES`), et aussi, par la règle d'accès ptrace du noyau, pour une cible qui détient des capacités que l'agent n'a pas |
| `--security-opt apparmor=unconfined` | le profil `docker-default` de l'agent n'autorise la lecture ptrace que vers des pairs `docker-default` : une cible `--privileged` ou `apparmor=unconfined` tourne `unconfined`, et `/proc/<pid>/root` renvoie `EACCES` (mesuré sur Ubuntu, `dmesg` : `apparmor="DENIED" operation="ptrace" profile="docker-default" … peer="unconfined"`). Avec ce flag, les deux cibles sont listées, sans évasion. Sans ce flag, ces cibles seules échouent : sur `EACCES` à l'ouverture de `/proc/<pid>/root`, l'agent renvoie `internal` avec un détail qui nomme les deux causes mesurées (`--cap-add SYS_PTRACE` absent, ou cible non confinée sans `--security-opt apparmor=unconfined`). Sur Fedora, sans effet (profil AppArmor vide), sans gêne non plus |
| SELinux : `--security-opt label=disable` | Docker l'applique déjà implicitement à un conteneur lancé avec `--pid=host` (observé : `ps -Z` → `spc_t`, `HostConfig.SecurityOpt=[label=disable]`, Docker 29.8.2 ; ce n'est pas un contrat documenté) : l'agent tourne en `spc_t`, lit `/proc/<pid>/root` de cibles `container_t`, `--privileged`, `label=disable` ou root `--privileged`, et se connecte au socket monté sans `:z` (mesuré sur Fedora 44, 16 cas sur 16, aucun AVC). Le flag explicite supprime cette dépendance ; mesuré sans gêne sur les deux familles (Ubuntu `d-*` 4/4, Fedora `c-*`/`d-*` 4/4). Aucune autre option `label=…` (toute option `label=` désactive le comportement implicite) : un agent `container_t` est refusé dès le socket (`avc: denied { write } … scontext=…container_t… tcontext=…container_var_run_t… tclass=sock_file`) |

Pas de `--privileged`. L'agent est déjà root sur l'hôte de fait (§7) : lever son confinement AppArmor, ou tourner en `spc_t` sous SELinux, ne change pas ce qu'il peut faire, seulement les cibles qu'il peut lire. Sur un hôte sans AppArmor (Fedora), `--security-opt apparmor=unconfined` est accepté et sans effet : une seule commande pour Ubuntu et Fedora. Podman et Docker rootless : non mesurés.

**Contrôle au démarrage** : un espace de noms cgroup privé (`--cgroupns=host` oublié) ferait répondre `container_changed` à chaque ouverture `procfs`, sans explication. Au démarrage, l'agent lit `/proc/self/cgroup` : `0::/` (racine d'un espace de noms privé) → avertissement visible dans les logs, et toute erreur `procfs` qui en découle porte le détail « lancez l'agent avec --cgroupns=host ».

**Alternative** : binaire des releases GitHub + service systemd, lancé en root. Binaire lancé en root hors conteneur : vérifié sur Ubuntu et Fedora ; service systemd non testé.

**Prérequis hôte** : Linux amd64 ou arm64, noyau 5.6+, Docker Engine avec API 1.44+, stockage overlay2 ou containerd.

**Développement sur macOS** : Docker Desktop fait tourner Docker dans une machine virtuelle Linux ; `/proc/<pid>/root` n'existe que dans cette machine. L'agent se lance donc dans un conteneur pendant le développement. `scripts/dev-agent.sh` compile le binaire Linux sur le Mac et le lance avec les droits de la commande recommandée ; `--cgroupns=host` est indispensable sur Docker Desktop (pilote cgroupfs). Les tests Linux passent par `scripts/test-linux.sh` : conteneur `golang` en root, mêmes droits, socket Docker monté. L'interface tourne en `vite dev` sur le Mac, avec un proxy vers l'agent.

## 9. Interface

- Page `/servers` : affiche le seul serveur de la v0.1 (cet agent) et un bouton « Ajouter un serveur » désactivé, marqué « bientôt ». Elle prépare l'interface à la gestion de plusieurs serveurs : un agent par serveur, l'interface en contacte plusieurs. L'API de l'agent n'a donc pas besoin de changer.
- Pages : `/login`, `/` (liste des conteneurs), `/c/[id]` avec quatre onglets : **Fichiers** (arborescence + aperçu + édition, marques du diff F5 dans l'arbre), **Changements** (liste F5), **Logs**, **Radiographie**.
- Toute écriture passe par une boîte de dialogue de confirmation qui rappelle le conteneur et le chemin.
- En lecture seule (`/api/meta`), les boutons d'écriture n'apparaissent pas.
- Téléphone : consultation (liste, fichiers en lecture, logs, radiographie). Édition pensée pour l'ordinateur.
- Textes : `web/src/lib/i18n/fr.ts` et `en.ts`, mêmes clés ; un test vérifie que les deux ont les mêmes clés. Les codes d'erreur de l'API ont chacun une traduction.

## 10. Tests

- **Sécurité d'abord, avant le code** : `procfs` testé sur un dossier temporaire servant de racine, avec liens piégés (absolu vers `/etc/passwd`, `../../`, boucle, lien vers lien). Aucune lecture ne doit sortir de la racine. Pas de Docker nécessaire. Cas obligatoires :
  - `procfs` : nœud de périphérique et FIFO → `not_regular` ; la FIFO ne doit pas bloquer. Le test du nœud de périphérique demande `CAP_MKNOD` : il tourne dans le job CI privilégié, ou se saute avec un message explicite.
  - `fstatfs` : avec `/` comme racine, `/proc/kallsyms` et un lien vers `/proc` → `forbidden_path`.
  - `tarindex`, sur des tar en mémoire : lien relatif depuis le parent, lien absolu, `..` bloqué à la racine, chaîne de liens, lien sur un dossier intermédiaire, `..` après un dossier lié (parent de la cible résolue), lien physique (type fichier, taille et mode de la cible, `Open` lit la cible), boucle arrêtée par la limite de sauts, progression plafonnée à 99 %, plafond octets / durée → `too_large`.
  - `tarindex`, binds : `Inspect.Mounts` avec un bind de `/`, `/proc`, `/sys`, de `/dev` exactement ou d'un chemin sous `/proc` ou `/sys` → `forbidden_path` qui nomme le bind, sans appel à `GetArchive` ; un bind ordinaire (ex. `/srv/data`) ou d'un seul périphérique (`/dev/log`) reste indexé ; un volume nommé `local` avec `o=bind` + `device=/proc` est traité comme un bind.
  - `CgroupHasID` : sous-chaîne refusée, tout composant `..` refusé, `docker-A.scope/docker-<B>.scope` refusé pour B, `docker-<id>.scope/init` accepté (docker:dind), `docker/<id>` accepté, `/<parent>/<id>` sans composant `docker` refusé (limite cgroupfs).
  - Conversion de `Mode` : setuid, setgid, sticky → `fs.ModeSetuid`, `fs.ModeSetgid`, `fs.ModeSticky`.
- **Unitaires Go** : `tarindex` sur des tar générés en mémoire ; `files` (lecture seule, `If-Match`, ordre audit → écriture → audit, contrôles de l'étape 2 : lien final ou lien dans le dossier parent → `link_in_path`, fichier non régulier → `not_regular`, dossier parent d'un envoi dans un pseudo-système → `forbidden_path` ; nom d'envoi `a/b`, `..`, `.` ou vide → `400 invalid_name` ; envoi sur un lien pendant → `link_in_path` ; `PutArchive` reçoit exactement le chemin nettoyé ; index retiré du cache après une écriture sur conteneur arrêté) avec un faux `dockerx` ; `secrets` ; `auth` (temps constant, limitation) ; `api` par `httptest` (codes d'erreur, `Origin`, en-têtes, SVG servi en texte).
- **Intégration** (tag `integration`) : vrai Docker sur GitHub Actions, conteneurs alpine et distroless ; matrice overlay2 / containerd configurée par `daemon.json`. Couvre F1 à F7 de bout en bout côté API, y compris conteneur arrêté.
- **Interface** : Vitest pour la logique (i18n, formatage, état du diff) ; un seul parcours Playwright : connexion → liste → ouvrir un fichier → voir le diff.
- **CI** : `go vet`, `go test ./...`, `npm run check`, Vitest, Playwright, build multi-architecture. Un **job privilégié** à part, sur le runner Ubuntu de GitHub Actions avec Docker : `sudo go test -tags integration ./...` (root : accès à `/proc/<pid>/root` des conteneurs, `CAP_MKNOD`), qui porte les tests d'intégration, le test du nœud de périphérique et les tests `procfs` qui demandent root ; ailleurs, ces tests se sautent avec un message explicite.

## 11. Spike (semaine 1)

Code jetable dans `spike/`, supprimé ensuite ; il reste dans l'historique git (parent du commit 72e1b50, branche `spike/semaine-1`) ; ses résultats sont dans le document de résultats. Quatre questions :

| # | Question | Réussite | Si échec | Résultat |
|---|---|---|---|---|
| 1 | Lister `/usr` d'une image distroless avec la séquence complète de §5.1 (pidfd, cgroup, `OpenatInRoot`, `Reopen`) | 1 000 entrées en moins d'1 s ; le test d'évasion par lien échoue comme prévu ; l'ID du conteneur apparaît dans `/proc/<pid>/cgroup` vu depuis l'agent conteneurisé | revoir §5.1 avant le socle | réussi avec `--cgroupns=host` : 1 000 entrées en 1 à 15 ms (Docker Desktop), aucune évasion ; sans ce flag, Docker Desktop donne `/../<id>`, refusé |
| 2 | `/changes` sur overlay2 et sur containerd | mêmes manipulations, mêmes résultats | F5 marqué « non pris en charge » sur le stockage en défaut en v0.1, avec message | réussi : résultats identiques sur les deux |
| 3 | Droits minimaux sur Ubuntu (AppArmor) et Fedora (SELinux) | une commande `docker run` sans `--privileged` qui marche sur les deux | documenter l'option nécessaire (`--security-opt`) ou recommander le binaire + systemd sur la distribution concernée | Ubuntu réussi : minimum mesuré `--pid=host --cap-add SYS_PTRACE` pour des cibles `docker-default`, `--cgroupns=host` requis par le socle partout ; cibles `--privileged` ou `apparmor=unconfined` : `EACCES` (refus AppArmor `ptrace`) sauf avec `--security-opt apparmor=unconfined` sur l'agent (n°3b) (§8) ; binaire en root hors conteneur vérifié, service systemd non testé. Fedora (SELinux `enforcing`) réussi : même minimum, `--security-opt label=disable` ajouté explicitement par prudence (sans effet mesuré ; Docker le pose déjà sur un conteneur `--pid=host`, l'agent tourne en `spc_t`) ; `apparmor=unconfined` sans effet ; les 4 cibles non confinées OK (n°3b) ; binaire root hors conteneur vérifié. Une seule commande `docker run` pour les deux familles (§8) |
| 4 | Sur un conteneur arrêté : `HEAD` et `GET /archive` répondent-ils, et `GET /archive` sur `/` inclut-il les volumes ? | réponses oui ou non | volumes absents → l'index affiche les points de montage vides avec une mention | oui et oui ; `GET` ne suit pas un lien final, `tarindex` résout les liens (§5.2) |

Le spike confirme aussi le chemin d'import du SDK Docker (`github.com/moby/moby/client`) et l'API de `filepath-securejoin` v0.7.0 (`pathrs-lite` : `OpenatInRoot`, `Reopen`) ; voir §3.

**Résultats détaillés** : [2026-10-07-resultats-spike.md](../spikes/2026-10-07-resultats-spike.md). Le code jetable du spike a été supprimé une fois tous les résultats consignés ; il est conservé dans l'historique git et dans ce document de résultats.

## 12. Planning

Base 5 à 10 h par semaine. 12 semaines maintenues : le code du spike a été écrit par des sous-agents, son rythme ne mesure pas celui de l'utilisateur en Go. Re-mesurer après la première semaine du socle et recaler à ce moment-là.

| Phase | Semaines | Contenu | Jalon |
|---|---|---|---|
| Spike | 1 | §11 | feu vert technique |
| Socle | 2 à 4 | dépôt, CI, agent, auth, audit, F1, squelette interface, page `/servers` | |
| Explorateur | 5 à 9 | F2, F3, F4, F5 | démo : un diagnostic complet sans terminal |
| Observation | 10 | F6, F7 | |
| Lancement | 11 à 12 | docs FR/EN, GoReleaser, image GHCR, release | v0.1 publique |

## 13. Risques

| Risque | Parade |
|---|---|
| AppArmor ou SELinux bloquent `/proc/<pid>/root` dans un conteneur | Ubuntu (AppArmor) : cibles `docker-default`, rien à changer (spike n°3) ; cibles `--privileged` ou `apparmor=unconfined`, refus AppArmor `ptrace`, levé par `--security-opt apparmor=unconfined` sur l'agent, dans la commande recommandée (spike n°3b, §8). Fedora (SELinux `enforcing`) : levé par le flag explicite `--security-opt label=disable` (Docker le pose déjà pour `--pid=host`, l'agent tourne en `spc_t`, toutes les cibles mesurées lisibles, aucun AVC : spike n°3 et 3b). Repli : binaire lancé en root hors conteneur (vérifié sur Ubuntu et Fedora ; service systemd non testé) |
| Construction de l'index longue sur une grosse image | progression affichée ; cache par conteneur |
| `/changes` incomplet sur containerd | risque levé : résultats identiques sur overlay2 et containerd (spike n°2) ; la matrice des tests d'intégration le garde sous contrôle |
| Rythme d'apprentissage de Go plus lent que prévu | re-mesure après la première semaine du socle ; tâches petites |
| Faille de lecture hors conteneur | tests d'évasion écrits avant le code ; `pathrs-lite` (filepath-securejoin) obligatoire ; revue de toute PR qui touche `fsview` |
| Pseudo-système de fichiers absent de la liste `fstatfs` (§5.1 étape 9) | liste de 15 types revue à chaque nouvelle version de noyau ciblée ; les fichiers non réguliers restent refusés quoi qu'il arrive |
| Montage FUSE appartenant au conteneur : un serveur FUSE qui ne répond pas bloque indéfiniment la goroutine de l'agent qui lit ou liste | délais sur les lectures et listings `procfs` côté API (la requête échoue, la goroutine reste bloquée) ; nombre de lectures simultanées par conteneur borné ; à mesurer dans le socle |
| Ancrage cgroup : une disposition de cgroups non prévue (ex. cgroupfs avec `--cgroup-parent`) est refusée | ancrage sur le composant le plus externe de forme ID Docker, cgroups enfants acceptés (§5.1 étape 3) ; tests dédiés (§10) ; refus → `container_changed`, limite documentée |
| Conteneur arrêté avec un bind de `/`, `/proc`, `/sys`, de `/dev` entier ou d'un chemin en dessous de `/proc` ou `/sys` de l'hôte (supervision) | explorateur indisponible pour ce conteneur, `forbidden_path` qui nomme le bind (§5.2 étape 7) ; index plafonné en octets et en durée (`too_large`) |
| Bind d'un dossier hôte contenant un sous-montage `/proc` ou `/sys` (rbind) | conteneur actif : les sous-montages présents dans le conteneur sont vus par `mountinfo` et refusent tout tar (§5.4). Avec la propagation `rprivate` par défaut, un montage ajouté **plus tard** sur l'hôte sous la source n'apparaît pas dans le conteneur et n'est pas détecté, alors que dockerd pourrait le voir en remontant le bind. Conteneur arrêté : non détecté en v0.1 (`Inspect.Mounts` ne montre que la `Source`) |
| Montage FUSE ou pseudo-système sous un dossier téléchargé : dockerd le lit pour construire le tar et peut rester bloqué, conteneur verrouillé | refus avant `GetArchive` par `Inspect.Mounts` et `mountinfo` (§5.4) ; nos propres lectures restent sous la garde (4 par conteneur, 10 s) |
| Course entre le contrôle des montages et `GetArchive` (conteneur actif) | Monter quoi que ce soit dans un conteneur actif exige `CAP_SYS_ADMIN` dans le conteneur (conteneur privilégié, ou `--cap-add SYS_ADMIN` : par exemple rclone ou s3fs avec `/dev/fuse`), ou root sur l'hôte (propagation `rshared`) ; limite assumée pour la v0.1 ; dockerd archive à partir des montages configurés ; règle « tout le conteneur » (§5.4) ; à remesurer sur une VM Ubuntu |
| Noms de fichiers non UTF-8 : JSON les transporte avec U+FFFD, le fichier ne s'ouvre plus depuis l'interface (`not_found`) | limite v0.1 ; le listing ne casse pas |
| Système de fichiers réseau « dur » (NFS…) sous un dossier téléchargé : non refusé, et dockerd peut rester bloqué comme avec un FUSE silencieux | limite assumée pour la v0.1 |
| Table des montages sans la racine `/` ou sans une destination d'`Inspect.Mounts` (racine du process déplacée par un chroot) | tar refusé (`forbidden_path`, §5.4) ; l'explorateur montre alors la vue de la racine du PID 1 |

## 14. Décisions hors technique

- Plusieurs serveurs : page `/servers` désactivée dès la v0.1 (§9), aucune préparation côté agent.
- Modèle à terme : gratuit, puis une version payante pour les équipes. Aucun impact sur la v0.1.
