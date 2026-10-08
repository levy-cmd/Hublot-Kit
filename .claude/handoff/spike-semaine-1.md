---
branche: spike/semaine-1
base: 97eafb9
de: levy (avec Claude)
le: 2026-10-07
repris_par:
verif: aucune — branche de documentation seulement (le code du spike est supprimé, il vit dans l'historique, parent de 72e1b50)
---

## Objectif
Répondre aux 4 paris techniques de Hublot-Kit avant tout code définitif, et reporter les réponses dans la spec v0.1.

## Fait
- Résultats prouvés (lignes RESULT réelles) — `docs/superpowers/spikes/2026-10-07-resultats-spike.md`
- Spec mise à jour, fait foi pour le socle — `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md` (§5 accès fichiers, §6 codes d'erreur, §8 installation, §10 tests, §13 risques)
- Branche poussée ; PR à ouvrir vers `main` : https://github.com/levy-cmd/Hublot-Kit/pull/new/spike/semaine-1 (`gh` absent sur la machine de départ)

## Décisions
- Agent : `--pid=host --cgroupns=host --cap-add SYS_PTRACE --security-opt apparmor=unconfined --security-opt label=disable` — mesuré sur Docker Desktop, Ubuntu (AppArmor), Fedora (SELinux) ; spec §8
- Lecture : fichiers réguliers seulement + refus `fstatfs` des pseudo-fs sur le chemin résolu — un `mknod` dans le conteneur lisait le disque de l'hôte (démontré)
- Écriture et téléchargement de dossier : chemin sans aucun lien (`openat2` + `RESOLVE_NO_SYMLINKS`) sinon `link_in_path` — spec §5.3
- Conteneur arrêté : l'index tar résout les liens lui-même, car `GET /archive` ne suit pas un lien final — spec §5.2
- SDK Docker : `github.com/moby/moby/client` (`docker/docker` s'arrête à v28)
- Planning non recalé : des sous-agents ont codé le spike, il ne mesure pas le rythme Go de levy — remesurer après la 1re semaine du socle (§12)

## Impasses
- Contrôle cgroup sous espace de noms privé : `/../<id>` (cgroupfs) ou `/../docker-<id>.scope` (systemd) — d'où `--cgroupns=host` obligatoire et tout `..` refusé
- Ancrage cgroup sur le *dernier* composant : casse docker-in-docker (`…/docker-<id>.scope/init`) — ancrer sur le composant Docker le plus externe
- Compter sur le `label=disable` implicite de Docker pour `--pid=host` : observé, non documenté — le flag est explicite
- Agent sous `docker-default` vers une cible privilégiée : refus AppArmor `ptrace` — d'où `apparmor=unconfined`
- Miroir Fedora automatique de Lima : coupe la connexion — télécharger l'image depuis `dl.fedoraproject.org`, vérifier le sha256, `limactl edit --set '.images=…'`

## Prochain geste
Écrire `docs/superpowers/plans/<date>-socle.md` à partir de la spec (§4 modules, §6 API, §7 sécurité, §10 tests, §12 phase Socle : dépôt, CI, agent, auth, audit, F1, squelette interface, page `/servers`), en tâches que levy peut coder lui-même pour apprendre Go.

## Pièges
- Le code `/proc` ne tourne que sous Linux : sur macOS, l'agent tourne dans un conteneur (Docker Desktop) ou dans les VM Lima `hk-ubuntu` / `hk-fedora` (arrêtées, `limactl start`)
- `ausearch -m avc` peut ne rien trouver alors que `/var/log/audit/audit.log` contient l'AVC (Fedora)
- Le test du nœud de périphérique exige `CAP_MKNOD` : job CI privilégié (§10)

## Pré-requis
skills : superpowers (writing-plans, subagent-driven-development), codex | variables d'env : aucune | services : Docker 29+, Lima (VM `hk-ubuntu`, `hk-fedora`), Go, Node 24
