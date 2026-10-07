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
| Écriture | `PUT /containers/{id}/archive` uniquement | Docker résout le chemin ; marche sur conteneur arrêté ; aucune écriture ne dépend de notre code anti-liens |
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
    fsview.go          type FS interface { List, Stat, Open } + types Entry
    procfs.go          conteneur actif
    tarindex.go        conteneur arrêté
  files/             seul point d'écriture : lecture seule, If-Match, audit, PutArchive
  secrets/           masquage des variables d'environnement
  auth/              jeton, sessions, limitation des tentatives
  audit/             journal JSONL en ajout seul
  api/               routes HTTP, SSE, en-têtes de sécurité, vérification d'Origin
  web/               go:embed du build SvelteKit
web/                 application SvelteKit
spike/               code jetable de la semaine 1 (supprimé après le spike)
```

**Frontières**

- `api` ne parle qu'à `dockerx`, `fsview`, `files`, `secrets`, `auth`, `audit`. Jamais au SDK Docker directement.
- `fsview.FS` :

  ```go
  type Entry struct {
      Name       string
      Type       EntryType // file, dir, symlink, other
      Size       int64
      Mode       fs.FileMode
      UID, GID   int
      ModTime    time.Time
      LinkTarget string    // brut, jamais résolu pendant le listing
  }

  type FS interface {
      List(ctx context.Context, dir string) (entries []Entry, truncated bool, err error)
      Stat(ctx context.Context, path string) (Entry, error)
      Open(ctx context.Context, path string) (io.ReadCloser, error)
  }
  ```

  Un constructeur `fsview.For(ctx, docker, containerID)` renvoie `procfs` si le conteneur tourne, `tarindex` sinon. Le reste du code ignore d'où viennent les fichiers.
- `dockerx.Client` est une interface : les tests unitaires utilisent un faux, les tests d'intégration un vrai Docker.

## 5. Accès aux fichiers

### 5.1 Conteneur actif (`procfs`)

Comparer deux `Inspect` ne suffit pas à prouver que la racine ouverte est celle du conteneur : un PID peut être réattribué avant que Docker ne voie l'arrêt. On accroche donc l'identité du process avec un **pidfd**, qui désigne un process précis et ne peut pas être réattribué :

1. `Inspect` → `State.Pid`. `Pid == 0` → bascule sur `tarindex`.
2. `pidfd_open(pid)` → `pidfd`. Échec (process disparu) → `container_changed`.
3. Lire `/proc/<pid>/cgroup` : l'ID complet du conteneur (64 caractères) doit y figurer comme **composant entier** du chemin, sous la forme `docker-<id>.scope` ou `docker/<id>`, sinon `container_changed`. Une sous-chaîne ne suffit pas. Tout chemin qui contient un composant `..` est refusé, quelle que soit la forme (`/../<id>`, `/../docker-<id>.scope`…) : l'agent tourne toujours avec `--cgroupns=host` (§8) ou sur l'hôte. Ce contrôle prouve que le process épinglé par `pidfd` appartient au conteneur. Dans l'implémentation du socle, ancrer sur le **dernier** composant : sinon un conteneur à qui on a délégué ses cgroups pourrait créer `docker-A.scope/docker-<B>.scope` et passer pour B si le PID de B lui était réattribué (cas très étroit, le contrôle `pidfd` de l'étape 5 reste requis).
4. Ouvrir `/proc/<pid>/root` en `O_PATH | O_DIRECTORY` → `rootfd`.
5. `pidfd_send_signal(pidfd, 0)` doit réussir : le process est toujours vivant, donc le PID n'a pas été réattribué entre les étapes 2 et 4, et `rootfd` est bien sa racine. Sinon → fermer, `container_changed`.
6. Ouvrir le chemin demandé par `pathrs.OpenatInRoot(rootfd, chemin)` (`github.com/cyphar/filepath-securejoin/pathrs-lite` v0.7.0, paquet `pathrs` ; le paquet racine n'exporte plus ces fonctions), qui renvoie un descripteur `O_PATH`. Puis, dans cet ordre :
   - **Pseudo-système** : `fstatfs` du descripteur `O_PATH`, **avant tout `Reopen`**, pour un dossier comme pour un fichier, et aussi pour `Stat` → `forbidden_path` (étape 9).
   - **Dossier** : `pathrs.Reopen(handle, O_RDONLY|O_DIRECTORY)`, pour lister uniquement.
   - **Fichier** (`Open`, aperçu, téléchargement) : `fstat` du descripteur `O_PATH` ; tout ce qui n'est pas `S_IFREG` (périphérique, FIFO, socket) est refusé **avant** `Reopen` (`not_regular`). Puis `pathrs.Reopen(handle, O_RDONLY|O_NONBLOCK)`. Raison : root dans un conteneur peut créer par `mknod` un nœud de périphérique de l'hôte, et l'agent est hors du cgroup devices du conteneur : l'ouvrir lirait le périphérique de l'hôte.

   **Interdits** : `filepath.Join`, `os.Open` sur un chemin construit à la main, tout suivi de lien hors `pathrs`.
7. `List` : sur le dossier rouvert, lire les noms, puis pour chaque nom `fstatat(dirfd, nom, AT_SYMLINK_NOFOLLOW)` et, pour un lien, `readlinkat(dirfd, nom)`. Tout se fait relativement au descripteur du dossier, avec un seul composant de chemin : aucun lien n'est suivi et rien ne peut sortir de la racine. Au-delà de 10 000 entrées : tronquer, `truncated = true`.
8. Navigation sur un lien : l'interface demande le chemin cible, résolu par `pathrs.OpenatInRoot` comme n'importe quel autre chemin.
9. **Pseudo-systèmes de fichiers non parcourables** (`/proc`, `/sys` et assimilés) : le listing de `/` les montre comme points de montage, on n'y entre pas. Raison : leurs pseudo-fichiers sont lus avec les privilèges hôte de l'agent (ex. `/proc/kallsyms` donne les vraies adresses du noyau). Le contrôle porte sur le **descripteur résolu**, pas sur le texte du chemin, donc un lien qui y mène est refusé aussi : `fstatfs` du descripteur `O_PATH` (étape 6) ; `f_type` parmi les 13 types ci-dessous → `forbidden_path`. S'applique à `List`, `Stat` et à la lecture d'un fichier.
   - Dans `golang.org/x/sys/unix` (v0.48.0) : `PROC_SUPER_MAGIC`, `SYSFS_MAGIC`, `CGROUP_SUPER_MAGIC`, `CGROUP2_SUPER_MAGIC`, `DEBUGFS_MAGIC`, `TRACEFS_MAGIC`, `SECURITYFS_MAGIC`, `BPF_FS_MAGIC`, `PSTOREFS_MAGIC`, `EFIVARFS_MAGIC`, `BINFMTFS_MAGIC`.
   - Constantes locales, absentes de x/sys : `CONFIGFS_MAGIC = 0x62656570` (`include/uapi/linux/magic.h` du noyau actuel ; absente des en-têtes 6.12) et `FUSE_CTL_SUPER_MAGIC = 0x65735543` (`fs/fuse/control.c`, aucun en-tête uapi).

   `/dev` reste parcourable : le lister est sans danger, et ses nœuds de périphérique sont déjà refusés par la règle des fichiers réguliers (étape 6, `not_regular`).

Spike : avec `--cgroupns=host`, l'agent conteneurisé lit `0::/docker/<id>` (Docker Desktop, pilote cgroupfs) ou `0::/system.slice/docker-<id>.scope` (Ubuntu, pilote systemd) ; le binaire sur l'hôte marche aussi. Sans `--cgroupns=host`, Docker Desktop donne `0::/../<id>`, refusé. 1 000 entrées listées en 1 à 15 ms (Docker Desktop ; Ubuntu : 3 × 1 000 entrées OK, durées non extraites). Noyau requis pour `pidfd_open` : 5.3, déjà couvert par le 5.6+ exigé pour `openat2`.

### 5.2 Conteneur arrêté (`tarindex`)

1. Clé de cache : `containerID + State.FinishedAt`. Un redémarrage puis un arrêt changent la clé. Une écriture réussie sur le conteneur arrêté (§5.3) supprime son index du cache ; il est reconstruit au prochain accès.
2. Construction en un seul exemplaire par clé (verrou de type singleflight). Pendant la construction, `List`, `Stat`, `Open` et le téléchargement d'un dossier renvoient `ErrIndexing` avec la progression (`indexing`, 202).
3. `GetArchive("/")` lu en flux par `archive/tar` ; seuls les en-têtes sont gardés, le contenu est lu et jeté. Progression = octets lus / `SizeRootFs` (`Inspect` avec `size=true`).
4. Index en mémoire : `map[dir][]Entry`. Au plus 3 index en cache (LRU), pour l'objectif de 50 Mo au repos hors index actif.
5. **Résolution** : tout chemin demandé est résolu composant par composant dans l'index. Les composants `.` et vides sont ignorés ; une barre oblique finale exige que le résultat soit un dossier. Chaque composant est cherché ; un lien rencontré est résolu avant de continuer : cible relative depuis le dossier parent du lien, cible absolue depuis la racine du conteneur, `..` bloqué à la racine, puis le parcours reprend. **Un seul compteur de 40 sauts** pour toute la résolution, cibles imbriquées comprises : une cible qui passe elle-même par des dossiers liés ne le remet pas à zéro. Seul le chemin entièrement résolu, sans lien, est envoyé à `GET /archive` ; jamais un `linkname` tel quel. Raison : `GET /archive` ne suit pas un lien en dernier composant (il renvoie l'entrée du lien : typeflag `'2'`, `linkname` brut, taille 0). `HEAD /archive` n'est plus utilisé. `Stat`, `Open` et le téléchargement d'un dossier attendent donc l'index, comme `List`.
6. `Stat(path)` : entrée de l'index au bout de la résolution ; `Stat` suit donc les liens comme tout chemin résolu (c'est le listing qui montre les liens eux-mêmes, avec `LinkTarget`). `Open(path)` : `GetArchive(chemin résolu)` et lecture de l'unique entrée ; entrée caractère, bloc ou FIFO → `not_regular`. `Entry.LinkTarget` est toujours la cible brute : `linkname` de l'en-tête tar, jamais une cible résolue par le démon.
7. Les entrées sous `/proc` et `/sys` sont ignorées : le tar d'un conteneur arrêté ne contient aucun pseudo-système vivant, ce qui suffit à la place du contrôle `fstatfs` de §5.1. `/dev` est montré : ses en-têtes sont sans danger, et `Open` refuse les périphériques.

Spike : `GET /archive` de `/` sur un conteneur arrêté **inclut le contenu des volumes nommés** (Docker Desktop, Ubuntu containerd, Ubuntu overlay2) ; l'index les montre comme le reste. `HEAD` et `GET /archive` répondent sur un conteneur arrêté ; seul `GET` est gardé. Mesuré sur alpine : 523 entrées, ~8,9 Mo lus, 217 à 276 ms.

### 5.3 Écriture (`files`)

Ordre strict, dans une seule fonction :

1. Mode lecture seule → `read_only`.
2. Contrôler la cible par `fsview`. Édition : le dernier composant ne doit pas être un lien et la cible doit être un fichier régulier, sinon `not_regular`. Envoi : le dossier parent ne doit pas se résoudre dans un pseudo-système de fichiers (§5.1 étape 9), sinon `forbidden_path`. Conteneur arrêté : la résolution passe par l'index (§5.2), donc `indexing` tant qu'il se construit.
3. Relire le fichier par `fsview`, calculer son SHA-256, comparer à `If-Match` → `conflict` si différent. Pour un nouveau fichier (envoi), `If-None-Match: *` → `conflict` s'il existe.
4. `audit` : écrire l'intention.
5. Construire un tar d'une entrée qui reprend `Mode`, `UID`, `GID` du fichier existant (nouveau fichier : `0644`, uid/gid du dossier parent), puis `PutArchive` sur le dossier parent. `PutArchive` ne reçoit que le chemin canonique, sans lien, validé à l'étape 2 : dossier parent résolu, dernier composant fichier régulier ou nom nouveau. Tout chemin non résolu, ou un lien dans la chaîne du parent ou de la cible, est refusé.
6. `audit` : écrire le résultat. Conteneur arrêté : en cas de succès, son index est retiré du cache (§5.2).

Limite assumée : entre l'étape 2 et l'étape 5, un process du conteneur peut encore modifier le fichier, ou remplacer un composant par un lien après les contrôles de l'étape 2. Pour un seul utilisateur, c'est acceptable.

### 5.4 Téléchargement

- Fichier : `fsview.Open` (contrôles `not_regular` et `forbidden_path`), relayé en flux.
- Dossier, conteneur actif : résoudre le dossier par `pathrs.OpenatInRoot`, `fstatfs` du descripteur (`forbidden_path`, §5.1 étape 9), puis `GetArchive` sur le chemin résolu, celui dont le descripteur a passé le contrôle (un lien final est résolu d'abord, jamais envoyé tel quel), relayé en `.tar` sans mise en mémoire.
- Dossier, conteneur arrêté : chemin résolu par l'index (§5.2, `indexing` pendant la construction), puis `GetArchive(chemin résolu)` relayé de même.
- Toujours `Content-Disposition: attachment`.

## 6. API

Préfixe `/api`, JSON. Erreur : `{"error": "<code>", "detail": "<texte libre pour les logs>"}`. Codes : `unauthorized`, `forbidden_origin`, `rate_limited`, `not_found`, `read_only`, `conflict`, `container_changed`, `indexing`, `too_large`, `not_regular` (pas un fichier régulier : périphérique, FIFO, socket), `forbidden_path` (le chemin résolu tombe dans un pseudo-système de fichiers, §5.1 étape 9), `docker_unavailable`, `internal`.

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
| GET | `/api/containers/{id}/fs/download?path=` | F3 ; `indexing` (conteneur arrêté) ; fichier : `not_regular`, `forbidden_path` ; dossier : `forbidden_path` |
| PUT | `/api/containers/{id}/fs/file?path=` | F4 édition, `If-Match` obligatoire ; `not_regular` (lien ou fichier non régulier), `forbidden_path`, `indexing` |
| POST | `/api/containers/{id}/fs/upload?dir=` | F4 envoi, multipart, `If-None-Match: *` ; `forbidden_path` (dossier dans un pseudo-système), `indexing` |
| GET | `/api/containers/{id}/changes` | F5 `[{path, kind: "added"|"modified"|"deleted"}]` |
| GET | `/api/containers/{id}/logs?tail=500` | F6 SSE : `{ts, stream: "stdout"|"stderr", line}` |
| GET | `/api/containers/{id}/logs/download` | F6 fichier texte |

Tout sauf `/api/login` et les fichiers statiques exige une session.

**Aperçu** : un octet nul dans les 8 premiers Ko → binaire, pas d'aperçu texte. Images : PNG, JPEG, GIF, WebP détectées par `http.DetectContentType`. **SVG et HTML sont toujours servis comme texte.**

## 7. Sécurité

L'agent a, de fait, les droits root sur l'hôte. Le README le dit en tête. Les mesures ci-dessous limitent les erreurs et l'exposition ; elles ne protègent pas contre un agent compromis.

- **Écoute** : `127.0.0.1:7878` par défaut. Adresse non locale sans TLS → avertissement visible au démarrage (cas légitime : dans un conteneur, ou derrière un reverse proxy).
- **Jeton** : 32 octets de `crypto/rand`, encodés en base64url, affichés une seule fois au premier lancement. Stocké sous forme d'empreinte SHA-256 dans `<data-dir>/token.sha256`. `hublot reset-token` en génère un nouveau. Comparaison par `subtle.ConstantTimeCompare`.
- **Tentatives** : 5 échecs par minute et par IP → `rate_limited` (429). Chaque échec est journalisé.
- **Session** : 32 octets aléatoires, en mémoire, 12 h. Cookie `HttpOnly`, `SameSite=Strict`, `Secure` si TLS, `Path=/`.
- **CSRF** : sur toute méthode autre que GET/HEAD, l'en-tête `Origin` doit être présent, bien formé, différent de `null`, et égal à l'**origine de confiance** comparée en entier (schéma, hôte, port), sinon `forbidden_origin`. Origine de confiance : `--public-origin` si défini (obligatoire derrière un reverse proxy, qui change souvent le schéma), sinon `http(s)://` + l'en-tête `Host` de la requête, le schéma venant de la présence de TLS sur l'agent. Les en-têtes `X-Forwarded-*` ne sont jamais crus. Les GET, flux SSE compris, ne modifient rien : le cookie `SameSite=Strict` suffit (les navigateurs n'envoient pas toujours `Origin` sur un GET de même origine, l'exiger casserait le flux).
- **En-têtes** : `Content-Security-Policy: default-src 'self'; img-src 'self' blob: data:; frame-ancestors 'none'`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`.
- **XSS par contenu de conteneur** : l'agent ne sert jamais un fichier de conteneur avec un type exécutable par le navigateur (HTML, SVG, XML, JavaScript). Règles :
  - L'aperçu ne renvoie que `text/plain; charset=utf-8` ou l'un des quatre types d'image matricielle de §6, toujours avec `nosniff`. Un SVG est un document actif : il n'est **jamais** rendu comme image, ni dans `<img>`, ni dans `<object>`, ni dans `<iframe>`. Il s'affiche comme texte.
  - Côté interface, le contenu d'un conteneur (fichiers, logs, noms de fichiers, variables d'env) passe toujours par l'échappement de Svelte. `{@html}` est interdit dans tout le code, et une règle ESLint le fait respecter.
  - Le téléchargement force `Content-Disposition: attachment` et `application/octet-stream`.
- **Secrets** : une variable est masquée si son nom contient (sans tenir compte de la casse) `PASSWORD`, `PASSWD`, `PASS`, `TOKEN`, `KEY`, `SECRET`, `CREDENTIAL`, `PRIVATE`, `DSN`, ou si sa valeur contient une URL avec identifiants (`://user:pass@`). Valeur masquée renvoyée : `null` avec `masked: true`.
- **Lecture seule** : `--read-only` ; appliqué dans `files` et nulle part ailleurs (un seul point d'écriture).
- **Journal d'audit** : `<data-dir>/audit.log`, une ligne JSON par événement : `{ts, action, container, path, result, error}`. Actions : `login.fail`, `env.reveal`, `file.write`, `file.upload`. Écrit avant et après chaque écriture.
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

**Installation recommandée**. Minimum mesuré par le spike sur Ubuntu (AppArmor actif, profil par défaut) : `--pid=host --cap-add SYS_PTRACE` + socket. `--cgroupns=host` est requis par le socle partout où l'agent tourne en conteneur, car tout `..` est refusé (§5.1 étape 3) ; il n'est inutile que pour le binaire lancé sur l'hôte :

```sh
docker run -d --name hublot-kit \
  --pid=host --cgroupns=host --cap-add SYS_PTRACE \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v hublot-data:/data \
  -p 127.0.0.1:7878:7878 \
  ghcr.io/levy-cmd/hublot-kit:0.1
```

| Droit | Raison |
|---|---|
| `--pid=host` | voir `/proc/<pid>` des conteneurs |
| `--cgroupns=host` | requis partout : avec un espace de noms cgroup privé, l'agent lit `/../<id>` (pilote cgroupfs, Docker Desktop) ou `/../docker-<id>.scope` (pilote systemd, Ubuntu), et le socle refuse tout composant `..` (§5.1 étape 3). Le spike passait sans ce flag sur Ubuntu parce qu'il acceptait encore `..` |
| `--cap-add SYS_PTRACE` | obligatoire pour une cible qui tourne sous un autre uid : sans lui, `/proc/<pid>/root` renvoie `EACCES` |

Pas de `--privileged`, aucun changement AppArmor. Fedora (SELinux) : en attente.

**Alternative** : binaire des releases GitHub + service systemd, lancé en root. Binaire lancé en root hors conteneur : vérifié sur Ubuntu ; service systemd non testé.

**Prérequis hôte** : Linux amd64 ou arm64, noyau 5.6+, Docker Engine avec API 1.44+, stockage overlay2 ou containerd.

**Développement sur macOS** : Docker Desktop fait tourner Docker dans une machine virtuelle Linux ; `/proc/<pid>/root` n'existe que dans cette machine. L'agent se lance donc dans un conteneur pendant le développement (`docker compose` avec `pid: host`, `cgroup: host`, `cap_add: [SYS_PTRACE]` et le code monté ; `cgroup: host` est indispensable sur Docker Desktop, pilote cgroupfs), l'interface en `vite dev` sur le Mac avec un proxy vers l'agent.

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
  - `tarindex`, sur des tar en mémoire : lien relatif depuis le parent, lien absolu, `..` bloqué à la racine, chaîne de liens, lien sur un dossier intermédiaire, boucle arrêtée par la limite de sauts.
  - `CgroupHasID` : sous-chaîne refusée, tout composant `..` refusé, `docker-A.scope/docker-<B>.scope` refusé une fois l'ancrage sur le dernier composant fait.
- **Unitaires Go** : `tarindex` sur des tar générés en mémoire ; `files` (lecture seule, `If-Match`, ordre audit → écriture → audit, contrôles de l'étape 2 : lien final → `not_regular`, dossier parent d'un envoi dans un pseudo-système → `forbidden_path`, index retiré du cache après une écriture sur conteneur arrêté) avec un faux `dockerx` ; `secrets` ; `auth` (temps constant, limitation) ; `api` par `httptest` (codes d'erreur, `Origin`, en-têtes, SVG servi en texte).
- **Intégration** (tag `integration`) : vrai Docker sur GitHub Actions, conteneurs alpine et distroless ; matrice overlay2 / containerd configurée par `daemon.json`. Couvre F1 à F7 de bout en bout côté API, y compris conteneur arrêté.
- **Interface** : Vitest pour la logique (i18n, formatage, état du diff) ; un seul parcours Playwright : connexion → liste → ouvrir un fichier → voir le diff.
- **CI** : `go vet`, `go test ./...`, tests d'intégration, `npm run check`, Vitest, Playwright, build multi-architecture.

## 11. Spike (semaine 1)

Code jetable dans `spike/`, supprimé une fois les réponses écrites dans ce document. Quatre questions :

| # | Question | Réussite | Si échec | Résultat |
|---|---|---|---|---|
| 1 | Lister `/usr` d'une image distroless avec la séquence complète de §5.1 (pidfd, cgroup, `OpenatInRoot`, `Reopen`) | 1 000 entrées en moins d'1 s ; le test d'évasion par lien échoue comme prévu ; l'ID du conteneur apparaît dans `/proc/<pid>/cgroup` vu depuis l'agent conteneurisé | revoir §5.1 avant le socle | réussi avec `--cgroupns=host` : 1 000 entrées en 1 à 15 ms (Docker Desktop), aucune évasion ; sans ce flag, Docker Desktop donne `/../<id>`, refusé |
| 2 | `/changes` sur overlay2 et sur containerd | mêmes manipulations, mêmes résultats | F5 marqué « non pris en charge » sur le stockage en défaut en v0.1, avec message | réussi : résultats identiques sur les deux |
| 3 | Droits minimaux sur Ubuntu (AppArmor) et Fedora (SELinux) | une commande `docker run` sans `--privileged` qui marche sur les deux | documenter l'option nécessaire (`--security-opt`) ou recommander le binaire + systemd sur la distribution concernée | Ubuntu réussi : minimum mesuré `--pid=host --cap-add SYS_PTRACE`, `--cgroupns=host` requis par le socle partout (§8) ; binaire en root hors conteneur vérifié, service systemd non testé ; Fedora en attente |
| 4 | Sur un conteneur arrêté : `HEAD` et `GET /archive` répondent-ils, et `GET /archive` sur `/` inclut-il les volumes ? | réponses oui ou non | `HEAD` refusé → `Stat` lit l'index ; volumes absents → l'index affiche les points de montage vides avec une mention | oui et oui ; `GET` ne suit pas un lien final, `tarindex` résout les liens (§5.2) |

Le spike confirme aussi le chemin d'import du SDK Docker (`github.com/moby/moby/client`) et l'API de `filepath-securejoin` v0.7.0 (`pathrs-lite` : `OpenatInRoot`, `Reopen`) ; voir §3.

**Résultats détaillés** : [2026-10-07-resultats-spike.md](../spikes/2026-10-07-resultats-spike.md). `spike/` est gardé jusqu'au passage Fedora.

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
| AppArmor ou SELinux bloquent `/proc/<pid>/root` dans un conteneur | Ubuntu (AppArmor, profil par défaut) : rien à changer (spike n°3). SELinux : en attente du passage Fedora. Repli : binaire lancé en root hors conteneur (vérifié sur Ubuntu ; service systemd non testé) |
| Construction de l'index longue sur une grosse image | progression affichée ; cache par conteneur |
| `/changes` incomplet sur containerd | risque levé : résultats identiques sur overlay2 et containerd (spike n°2) ; la matrice des tests d'intégration le garde sous contrôle |
| Rythme d'apprentissage de Go plus lent que prévu | re-mesure après la première semaine du socle ; tâches petites |
| Faille de lecture hors conteneur | tests d'évasion écrits avant le code ; `pathrs-lite` (filepath-securejoin) obligatoire ; revue de toute PR qui touche `fsview` |
| Pseudo-système de fichiers absent de la liste `fstatfs` (§5.1 étape 9) | liste de 13 types revue à chaque nouvelle version de noyau ciblée ; les fichiers non réguliers restent refusés quoi qu'il arrive |
| Ancrage cgroup sur le dernier composant (reporté au socle) | test dédié (§10) ; contrôle `pidfd` de §5.1 étape 5 maintenu |

## 14. Décisions hors technique

- Plusieurs serveurs : page `/servers` désactivée dès la v0.1 (§9), aucune préparation côté agent.
- Modèle à terme : gratuit, puis une version payante pour les équipes. Aucun impact sur la v0.1.
