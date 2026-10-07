# Résultats du spike — 2026-10-07

- **Spec** : [2026-10-07-hublot-kit-v0.1-design.md](../specs/2026-10-07-hublot-kit-v0.1-design.md), §11
- **Branche** : `spike/semaine-1`
- **Machines** : Docker Desktop (macOS, Docker 29.7.2, API 1.55, pilote cgroupfs, stockage containerd `overlayfs`) ; VM Lima `hk-ubuntu` (AppArmor actif, profil `docker-default`, pilote cgroup systemd, stockage containerd puis overlay2) ; VM `hk-fedora` en attente.

**Temps passé** : le code du spike a été écrit par des sous-agents, entre 18:48 et 20:10 environ. Ce temps ne mesure pas le rythme de Go de l'utilisateur : pas de recalage du planning (§12), à re-mesurer après la première semaine du socle.

## Réponses

| # | Question | Réponse | Preuve (ligne RESULT recopiée) | Conséquence pour la spec |
|---|---|---|---|---|
| 1 | Listing /proc + securejoin sur distroless | Oui. 1 000 entrées en 1 à 15 ms, cible root comme nonroot (uid 65532) en `--read-only`, et après redémarrage (nouveau PID). Le lien absolu piégé ne sort pas ; la lecture naïve de contrôle, elle, sort. | `RESULT list /usr/many: 1000 entries (truncated=false) in 4.657083ms` ; `RESULT securejoin read evil: "" err=securejoin.OpenInRoot evil: openat2 container-root/evil: no such file or directory` ; `CONTROL naive read evil: "HOST-SECRET\n" err=<nil>` ; `RESULT restart pid 45641 -> 46002` (Docker Desktop, `--cgroupns=host`) | §5.1 confirmé. Ajouts : fichiers réguliers seulement, `/proc` `/sys` `/dev` non parcourables (voir « Autres constats »). |
| 1b | ID du conteneur dans /proc/<pid>/cgroup (agent conteneurisé) | Oui avec `--cgroupns=host` : `0::/docker/<id>` (Docker Desktop) ou `0::/system.slice/docker-<id>.scope` (Ubuntu). Sans : `0::/../<id>` sur Docker Desktop (pilote cgroupfs), refusé par le contrôle strict ; `0::/../docker-<id>.scope` sur Ubuntu (pilote systemd), accepté. | `container d0900b26711d pid=45641 cgroup="0::/docker/d0900b26711d7c9d3edea674a0882a65d9944777506646b9e44b5f1df6f10c7c"` ; sans `--cgroupns=host` : `FATAL open root: container changed: pid 46205 is not in the container's cgroup` | §8 : `--cgroupns=host` obligatoire. §5.1 étape 3 : ID complet comme composant entier, parent `..` refusé. |
| 2 | /changes identique sur containerd et overlay2 | Oui. Mêmes 5 lignes ; survit au redémarrage ; marche sur conteneur arrêté. | `A /added.txt`, `A /second-run.txt`, `C /etc`, `C /etc/profile`, `D /etc/issue` sur Docker Desktop (overlayfs), Ubuntu overlayfs et Ubuntu overlay2 ; `Diff result: IDENTIQUES` | F5 sans restriction de stockage. Risque §13 levé. |
| 3 | Droits minimaux Ubuntu | `--pid=host --cgroupns=host --cap-add SYS_PTRACE` + socket. Aucun changement AppArmor, pas de `--privileged`. `SYS_PTRACE` requis dès que la cible tourne sous un autre uid (EACCES sinon, pas de refus AppArmor dans `dmesg`). Binaire hôte + systemd : OK. | `OK     2-ptrace (--cgroupns=host --cap-add SYS_PTRACE)` ; `ECHEC  1-aucun (--cgroupns=host)` → `FATAL open root: open root of pid 15347: permission denied` (cible nonroot) ; `OK     6-binaire-hote (root, hors conteneur)` | §8 : commande d'installation mise à jour. |
| 3 | Droits minimaux Fedora (SELinux actif) | en attente (image en téléchargement) | en attente | en attente ; §8 et §13 marqués « en attente » |
| 4 | HEAD / GET archive sur conteneur arrêté | Oui, les deux répondent. Mais `GET /archive` ne suit pas un lien en dernier composant : il renvoie l'entrée du lien (typeflag `'2'`, `linkname` brut, taille 0). `HEAD` renvoie un `LinkTarget` résolu par le démon (absolu). Identique sur Docker Desktop, Ubuntu containerd, Ubuntu overlay2. | `RESULT HEAD /etc/os-release: {Name:os-release Size:21 Mode:134218239 Mtime:2026-09-17T17:32:21Z LinkTarget:/usr/lib/os-release} err=<nil>` ; `RESULT GET /etc/os-release: typeflag='2'(50) name="os-release" linkname="../usr/lib/os-release" size=0` ; `RESULT GET /usr/lib/os-release: first line "NAME=\"Alpine Linux\"" err=<nil>` | §5.2 étape 5 : `HEAD` gardé pour `Stat` ; `tarindex` résout les liens lui-même depuis l'index. |
| 4 | Volumes inclus dans GET /archive "/" | Oui, le contenu d'un volume nommé est dans l'archive de `/` d'un conteneur arrêté (Docker Desktop, Ubuntu containerd, Ubuntu overlay2). | `RESULT in-root.txt present=true, in-volume.txt (volume) present=true` | Plan B inutile : l'index montre le contenu des volumes. |
| 5 | API securejoin (OpenatInRoot / Reopen, paquet) | v0.7.0 : le paquet racine n'exporte plus que `SecureJoin`, `SecureJoinVFS`, `IsNotExist`, `VFS`. `OpenatInRoot` et `Reopen` sont dans `github.com/cyphar/filepath-securejoin/pathrs-lite` (paquet `pathrs`). Tire `cyphar.com/go-pathrs` v0.2.5 en indirect. | `go doc github.com/cyphar/filepath-securejoin OpenatInRoot` → `doc: no symbol OpenatInRoot in package github.com/cyphar/filepath-securejoin` ; sous-paquet : `func OpenatInRoot(root *os.File, unsafePath string) (*os.File, error)`, `func Reopen(handle *os.File, flags int) (*os.File, error)` | §3 et §5.1 étape 6 : `pathrs.OpenatInRoot`, `pathrs.Reopen`. |
| 6 | Import du SDK Docker | `github.com/moby/moby/client` (v0.6.1, qui tire `github.com/moby/moby/api` v1.56.1). `github.com/docker/docker` s'arrête à v28.5.2 et ne suit pas Docker 29. | `go list -m -versions github.com/moby/moby/client` → `… v0.5.1 v0.6.0 v0.6.1` ; `go list -m -versions github.com/docker/docker` → `… v28.5.1+incompatible v28.5.2+incompatible` | §3 : import `github.com/moby/moby/client` dans `internal/dockerx`. |

**Index tar mesuré** : 523 entrées, 8 947 712 octets lus jusqu'à la fin du tar, 216 à 276 ms pour une image alpine (266 et 276 ms Docker Desktop, 273 ms Ubuntu containerd, 217 ms Ubuntu overlay2).

## Autres constats (décisions du contrôleur)

| Constat | Décision | Section |
|---|---|---|
| Root dans un conteneur peut créer un nœud de périphérique par `mknod`. L'agent est hors du cgroup devices du conteneur : ouvrir ce nœud lit le périphérique de l'hôte. Démontré par un test (`TestReadFileRefusesDevice`). | `Open` / lecture sur fichiers réguliers seulement : `fstat` du descripteur `O_PATH`, refus hors `S_IFREG`, puis `Reopen` avec `O_NONBLOCK`. Dossiers ouverts pour lister seulement. | §5.1 |
| `/proc`, `/sys` du conteneur contiennent des pseudo-fichiers réguliers lus avec les privilèges hôte de l'agent (ex. `/proc/kallsyms` donne les vraies adresses du noyau). | `/proc`, `/sys`, `/dev` montrés comme points de montage, jamais parcourus, en `procfs` comme en `tarindex`. | §5.1, §5.2 |
| `CgroupHasID` par sous-chaîne accepterait un faux chemin. | ID de 64 caractères comparé comme composant entier (`docker-<id>.scope` ou `docker/<id>`). Reste ouvert : un conteneur à qui on délègue ses cgroups peut créer `docker-A.scope/docker-<B>.scope` ; ancrer sur le dernier composant dans le socle. | §5.1 |
| Sans `--cgroupns=host`, la forme du chemin cgroup dépend du pilote (cgroupfs : `/../<id>` ; systemd : `/../docker-<id>.scope`). | `--cgroupns=host` imposé partout, contrôle strict sans parent `..`. | §8 |

## En attente

- Question 3, Fedora (SELinux) : passage `q3-perms.sh` sur `hk-fedora` dès que l'image est téléchargée. `spike/` est gardé jusque-là ; la ligne Fedora de ce document, §8 et §13 de la spec seront complétés dans un commit suivant.
