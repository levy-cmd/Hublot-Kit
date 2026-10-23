# Hublot-Kit

> **Sécurité : installer Hublot-Kit revient à lui donner les droits root sur l'hôte.**
> Il parle au socket Docker et lit les fichiers de vos conteneurs. Gardez-le sur `127.0.0.1` et accédez-y par un
> tunnel SSH ou un reverse proxy HTTPS. Lisez d'abord [le modèle de sécurité](docs/fr/securite.md).

[Read in English](README.md)

Explorer l'intérieur des conteneurs Docker d'un serveur depuis un navigateur, sans terminal. Choisissez un conteneur
et voyez ses fichiers, ce qui a changé depuis sa création, ses logs et sa configuration.

![Liste des conteneurs](docs/images/containers.png)
![Explorateur de fichiers avec les changements depuis la création](docs/images/explorer.png)

## Ce qu'il fait (v0.1)

- **Conteneurs** : nom, image, statut, ports, durée de fonctionnement ; filtre et recherche.
- **Fichiers** : toute l'arborescence d'un conteneur actif ou arrêté, aperçu texte et image, taille, propriétaire,
  droits.
- **Téléchargement** d'un fichier, ou d'un dossier en archive tar.
- **Édition** d'un fichier texte ou **envoi** d'un fichier, après une confirmation qui rappelle le conteneur et le
  chemin.
- **Changements** : ce qui a été ajouté, modifié ou supprimé depuis la création du conteneur.
- **Logs** : flux en direct, recherche, pause et reprise, téléchargement.
- **Radiographie** : image, état, commande, environnement (secrets masqués, révélés un par un et journalisés), ports,
  réseaux, montages.
- **Lecture seule** (`--read-only`) : aucune écriture.

Un serveur, un utilisateur, une interface en français et en anglais, un seul binaire statique avec l'interface
embarquée, pas de base de données, aucune télémétrie. Les process et ressources, le démarrage et l'arrêt, et un
terminal web sont prévus pour la v0.2.

## Démarrage rapide

Prérequis : Linux amd64 ou arm64, noyau 5.6 ou plus récent (5.8 pour éditer les fichiers d'un conteneur actif),
Docker Engine avec l'API 1.44 ou plus récente, stockage overlay2 ou containerd. La même commande sert sur les hôtes
AppArmor (Ubuntu, Debian) et SELinux (Fedora, RHEL) ; [à quoi sert chaque option](docs/fr/installation.md#à-quoi-sert-chaque-option).

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

Le jeton d'accès s'affiche une seule fois, au premier lancement, sur la ligne
`Access token (shown only once): …` (les messages de l'agent sont en anglais).

```sh
docker logs hublot-kit
```

Depuis votre ordinateur, ouvrez un tunnel, puis allez sur <http://127.0.0.1:7878> et collez le jeton.

```sh
ssh -L 7878:127.0.0.1:7878 user@server
```

Vous préférez un binaire lancé par systemd ? Voir [Binaire et systemd](docs/fr/installation.md#binaire-et-systemd).

## Documentation

- [Visite de l'interface](docs/fr/guide.md) : la disposition, les panneaux, le thème, la lecture seule telle que l'interface la montre.
- [Installation](docs/fr/installation.md) : Docker, binaire et systemd, reverse proxy, jeton, mise à jour,
  désinstallation, vérification des signatures et des SBOM d'une release.
- [Modèle de sécurité](docs/fr/securite.md) : ce que l'agent peut faire, ce qui le protège, comment l'exposer.
- [Limites connues](docs/fr/limites.md) : ce que la v0.1 ne fait pas, ou ne détecte pas.

## Développement

Prérequis : Go 1.27 ou plus récent, Node 24 (22.17 ou plus récent), Docker. Le code Go qui touche `/proc` ne tourne
que sous Linux : `make test-linux` le lance en root dans un conteneur `golang`.

```sh
make test
make build
make test-linux
```

Sur macOS (Docker Desktop), `/proc/<pid>/root` n'existe que dans la machine virtuelle de Docker : l'agent tourne donc
dans un conteneur, `make web && scripts/dev-agent.sh`. L'interface en mode développement passe par un proxy vers
l'agent sur `127.0.0.1:7878` : `cd web && npm run dev`. La spec technique est dans
[docs/superpowers/specs](docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md).

## Licence

[MIT](LICENSE)
