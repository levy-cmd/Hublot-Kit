# Installer Hublot-Kit

> **Hublot-Kit est, de fait, root sur l'hôte.** Lisez [le modèle de sécurité](securite.md) avant d'installer, et
> [les limites connues](limites.md) avant de vous y fier.

## Prérequis

- Linux amd64 ou arm64.
- Noyau 5.6 ou plus récent pour lire les fichiers ; 5.8 ou plus récent pour éditer ou envoyer des fichiers dans un
  conteneur actif.
- Docker Engine avec l'API 1.44 ou plus récente (`docker version` l'indique), stockage overlay2 ou containerd.
- Non pris en charge en v0.1 : Docker rootless, `userns-remap`, Podman, Kubernetes.

## Docker (recommandé)

La même commande sert sur les hôtes AppArmor (Ubuntu, Debian) et SELinux (Fedora, RHEL). Elle a été mesurée sur
Ubuntu avec AppArmor et sur Fedora avec SELinux en mode `enforcing`.

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

`0.1` suit les correctifs 0.1.x ; `0.1.0` ne bouge jamais. Il n'y a pas d'étiquette `latest`.

### À quoi sert chaque option

| Option | Raison |
|---|---|
| `--pid=host` | voir `/proc/<pid>` des conteneurs : c'est par là que l'agent lit leurs fichiers. |
| `--cgroupns=host` | obligatoire : l'agent prouve qu'un process appartient à un conteneur par son chemin de cgroup. Dans un espace de noms cgroup privé, il ne le peut pas, et chaque requête de fichier répond `container_changed` ; l'agent affiche alors `WARNING: private cgroup namespace (/proc/self/cgroup = 0::/): …` au démarrage. |
| `--cap-add SYS_PTRACE` | lire les fichiers d'un conteneur qui tourne sous un autre utilisateur, ou qui détient des capacités que l'agent n'a pas. |
| `--security-opt apparmor=unconfined` | hôtes AppArmor : sans elle, les conteneurs lancés en `--privileged` ou `apparmor=unconfined` ne s'explorent pas (l'erreur nomme cette option). Sans effet ailleurs. |
| `--security-opt label=disable` | hôtes SELinux : permet à l'agent de lire les autres conteneurs et d'utiliser le socket Docker. Docker l'applique déjà à un conteneur lancé avec `--pid=host`, mais ce n'est pas un contrat documenté. N'utilisez aucune autre option `label=` : elle couperait l'agent du socket. Sans effet ailleurs. |
| `-v /var/run/docker.sock:…` | le seul accès de l'agent à Docker. Le monter en `:ro` ne protège rien. |
| `-v hublot-data:/data` | l'empreinte du jeton d'accès et le journal d'audit. |
| `-p 127.0.0.1:7878:7878` | joignable depuis le serveur seulement ; passez par un tunnel SSH ou un reverse proxy. |

`--privileged` n'est pas nécessaire. L'agent est déjà root sur l'hôte de fait : lever son confinement AppArmor, ou le
faire tourner en `spc_t` sous SELinux, change les conteneurs qu'il peut lire, pas ce qu'il peut faire.

### Première connexion

Le jeton d'accès s'affiche une seule fois, au premier lancement, sur la ligne
`Access token (shown only once): …` (les messages de l'agent sont en anglais).

```sh
docker logs hublot-kit
```

Depuis votre ordinateur, ouvrez un tunnel, allez sur <http://127.0.0.1:7878> et collez le jeton.

```sh
ssh -L 7878:127.0.0.1:7878 user@server
```

### Options

Chaque option a sa variable d'environnement. Avec Docker, ajoutez les options après le nom de l'image, ou les
variables avec `-e` avant lui.

| Option | Variable | Défaut |
|---|---|---|
| `--listen` | `HUBLOT_LISTEN` | `127.0.0.1:7878` (`0.0.0.0:7878` dans l'image) |
| `--data-dir` | `HUBLOT_DATA_DIR` | `/var/lib/hublot-kit` (`/data` dans l'image) |
| `--read-only` | `HUBLOT_READ_ONLY` | `false` : avec `true`, aucune écriture, et les boutons d'écriture disparaissent |
| `--tls-cert`, `--tls-key` | `HUBLOT_TLS_CERT`, `HUBLOT_TLS_KEY` | vides : servir directement en HTTPS avec un certificat que vous fournissez |
| `--public-origin` | `HUBLOT_PUBLIC_ORIGIN` | vide : l'URL publique derrière un reverse proxy |

`hublot --help` les liste ; `hublot --version` affiche la version.

## Derrière un reverse proxy HTTPS

Gardez le port sur `127.0.0.1`, faites pointer le proxy vers `http://127.0.0.1:7878`, et donnez à l'agent son origine
publique, par exemple `-e HUBLOT_PUBLIC_ORIGIN=https://hublot.mondomaine.fr` avant le nom de l'image. Sans elle, un
proxy qui change le schéma fait échouer toute requête autre que GET par `forbidden_origin`, connexion comprise, et le
cookie de session n'est pas marqué `Secure`.

L'onglet Logs est un flux Server-Sent Events : le proxy ne doit pas le mettre en tampon (l'agent envoie
`X-Accel-Buffering: no`, que nginx respecte) et doit accepter les lectures longues (le flux envoie un battement toutes
les 15 secondes). L'agent ne croit jamais les en-têtes `X-Forwarded-*`.

Sans proxy, `--tls-cert` et `--tls-key` servent directement en HTTPS. La v0.1 n'obtient pas de certificat elle-même.

## Binaire et systemd

L'alternative au conteneur : le binaire statique, lancé en root par systemd. Vérifiez d'abord l'archive
([Vérifier la release](#vérifier-la-release)). Utilisez `amd64` ou `arm64`.

```sh
V=0.1.0
ARCH=amd64
curl -fLO "https://github.com/levy-cmd/Hublot-Kit/releases/download/v$V/hublot-kit_${V}_linux_${ARCH}.tar.gz"
tar -xzf "hublot-kit_${V}_linux_${ARCH}.tar.gz"
sudo install -m 0755 hublot /usr/local/bin/hublot
sudo install -m 0644 hublot-kit.service /etc/systemd/system/hublot-kit.service
sudo /usr/local/bin/hublot reset-token
sudo systemctl daemon-reload
sudo systemctl enable --now hublot-kit
```

`hublot reset-token` avant le premier démarrage affiche le jeton dans votre terminal (`New access token (shown only once): …`). Démarré d'abord, l'agent
l'écrirait dans le journal du système, que lisent les membres des groupes `adm` ou `systemd-journal`.

Sur un hôte SELinux, `install` donne au binaire la bonne étiquette ; un binaire déplacé par `mv` depuis un dossier
personnel garde une étiquette que systemd refuse de lancer (`sudo restorecon /usr/local/bin/hublot` la corrige).

L'unité écoute sur `127.0.0.1:7878`, garde ses données dans `/var/lib/hublot-kit` (droits 0700) et tourne avec
`NoNewPrivileges`, `ProtectSystem=strict` et `PrivateTmp`. Elle ne masque pas `/proc` et ne retire aucune capacité :
l'explorateur en a besoin.

### Réglages

Changez les options par un fichier complémentaire (drop-in), jamais en modifiant l'unité :

```sh
sudo systemctl edit hublot-kit
```

```ini
[Service]
Environment=HUBLOT_PUBLIC_ORIGIN=https://hublot.example.com
Environment=HUBLOT_READ_ONLY=true
```

```sh
sudo systemctl restart hublot-kit
```

## Jeton d'accès

Un nouveau jeton invalide l'ancien une fois l'agent relancé ; d'ici là, un agent déjà lancé accepte encore l'ancien.
Après 5 échecs de connexion en une minute depuis une adresse, les connexions depuis cette adresse sont refusées
jusqu'à la fin de la minute.

Avec Docker :

```sh
docker exec hublot-kit /hublot reset-token
docker restart hublot-kit
```

Avec systemd :

```sh
sudo /usr/local/bin/hublot reset-token
sudo systemctl restart hublot-kit
```

## Dossier de données

`--data-dir` doit être dédié à Hublot-Kit. Il contient `token.sha256` (l'empreinte du jeton, jamais le jeton) et
`audit.log`. Absent, il est créé en 0700. Existant, ses droits ne sont pas modifiés, mais l'agent refuse de démarrer
si c'est un lien symbolique, s'il appartient à un autre utilisateur, ou s'il est accessible en écriture au groupe ou à
tous (`/tmp`, par exemple). Ses fichiers ne sont jamais ouverts à travers un lien symbolique. Le journal d'audit n'est
pas tourné en v0.1.

## Mise à jour

Avec Docker, tirez l'image, supprimez le conteneur et relancez la commande de [Docker](#docker-recommandé) ; le volume
`hublot-data` garde le jeton.

```sh
docker pull ghcr.io/levy-cmd/hublot-kit:0.1
docker rm -f hublot-kit
```

Avec systemd, téléchargez et vérifiez la nouvelle archive, puis remplacez le binaire et relancez :

```sh
sudo install -m 0755 hublot /usr/local/bin/hublot
sudo systemctl restart hublot-kit
```

## Désinstallation

Avec Docker :

```sh
docker rm -f hublot-kit
docker volume rm hublot-data
```

Avec systemd :

```sh
sudo systemctl disable --now hublot-kit
sudo rm -rf /usr/local/bin/hublot /etc/systemd/system/hublot-kit.service /etc/systemd/system/hublot-kit.service.d
sudo systemctl daemon-reload
sudo rm -rf /var/lib/hublot-kit
```

## Vérifier la release

Les releases sont construites et signées par GitHub Actions, avec des signatures [cosign](https://docs.sigstore.dev/)
sans clé (keyless), liées au workflow de release de ce dépôt. Il faut cosign 3.0 ou plus récent.

Ce que prouve une signature valide : le fichier ou l'image sort de ce workflow, lancé sur ce tag, et n'a pas changé
depuis. Ce qu'elle ne prouve pas : que le code du tag a été relu, ni qu'un SBOM est complet (il liste ce que syft,
pour les archives, et BuildKit, pour l'image, ont trouvé dans le binaire). Aucune provenance SLSA n'est publiée en
v0.1.

### Archives

`checksums.txt` donne le SHA-256 des archives et de leurs SBOM SPDX (`.sbom.json`) ; sa signature les couvre. Les
bundles `.sigstore.json` n'y figurent pas : chacun porte son propre certificat, contrôlé par `cosign verify-blob`.
Chaque archive a aussi son propre bundle.

```sh
V=0.1.0
ARCH=amd64
BASE="https://github.com/levy-cmd/Hublot-Kit/releases/download/v$V"
curl -fLO "$BASE/hublot-kit_${V}_linux_${ARCH}.tar.gz"
curl -fLO "$BASE/checksums.txt"
curl -fLO "$BASE/checksums.txt.sigstore.json"
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/levy-cmd/Hublot-Kit/.github/workflows/release.yml@refs/tags/v$V" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
sha256sum --ignore-missing -c checksums.txt
```

### Image

L'image est signée par son empreinte, pour linux/amd64 et linux/arm64, et porte une attestation SBOM.

```sh
cosign verify \
  --certificate-identity-regexp '^https://github\.com/levy-cmd/Hublot-Kit/\.github/workflows/release\.yml@refs/tags/v0\.1\.[0-9]+$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/levy-cmd/hublot-kit:0.1
docker buildx imagetools inspect ghcr.io/levy-cmd/hublot-kit:0.1 --format '{{ json .SBOM }}'
```

Pour lancer exactement l'image vérifiée, remplacez `:0.1` dans la commande de [Docker](#docker-recommandé) par
l'empreinte de l'image vérifiée : `cosign verify` affiche du JSON, où `critical.image.docker-manifest-digest` la
donne (`sha256:…`). La référence devient `ghcr.io/levy-cmd/hublot-kit@sha256:…`.
