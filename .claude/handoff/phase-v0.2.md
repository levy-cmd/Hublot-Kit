---
branche: phase/v0.2
base: 5b3c947
de: Livaille An-nour (session Claude principale)
le: 2026-10-23
repris_par:
verif: make test
---

## Objectif
Livrer la v0.2 (interface Graphite & Émeraude, Secrets, Images, Snapshots) en 10 tranches verticales : plan `docs/superpowers/plans/2026-10-23-v0.2.md`, suivi `.claude/handoff/phase-v0.2.registre.md`.

## Fait
- Tranches 1 à 5 validées (relecteur + Codex), consignées au registre (dernière : 7640785).
- Tranche 6 (fichiers d'image) : 853b601, validée par les deux revues ; suivi des Minor en 5b3c947, CI PR #10 verte (run 40020572507). **Pas encore consignée.**
- Audit de sécurité indépendant de 7640785 : 5 constats prouvés (2 Important, 3 Minor). Liste dans le prompt de l'agent v0.1.2, à recopier au registre à la clôture du correctif.
- Recherche concurrents (Portainer, Dozzle, Dockhand, Yacht, Lazydocker) faite, sources dans la conversation, non écrite dans le dépôt.

## Décisions
- Tranches séquentielles : disque du Mac presque plein, un seul Docker ; le parallèle 5/8 n'a pas été jugé rentable.
- Utilisateur (2026-10-23) : les 5 failles se corrigent sur `phase/v0.1.2` (depuis v0.1.1 = origin/main 6b30eab), puis sont reportées dans la v0.2. Tag, release et image v0.1.2 : ARRÊT, accord de l'utilisateur à chaque étape.
- Risque résiduel accepté en tranche 5 (`secrets.History`, marqueur de shell dans une valeur d'ARG) : à écrire dans les limites à la tranche 10.

## Impasses
- Reviews Codex = lecture seule, sans exécution ; les sondes prouvées viennent du relecteur.
- Une seule règle « clé `NOM=` » ne ferme pas `ARG` à valeur avec blanc : seule la règle « une affectation, en dernier mot » tient (registre, tranche 5).

## Prochain geste
1. Relancer l'agent v0.1.2 : worktree `.claude/worktrees/agent-a9e9962a8f58d5d98` (branche locale `phase/v0.1.2`, tests rouges 350222b, correctifs non commités dans 7 fichiers). Puis revues, PR brouillon pour la CI, report dans `phase/v0.2`.
2. Trancher `TestCost` (`internal/secrets/secrets_test.go`, 5b3c947) : Codex le juge affaibli (plafonds 5 s, ratio 20). Avis du relecteur interrompu par le redémarrage. Ensuite consigner la tranche 6.
3. Tranche 7 (diff de contenu avec l'image) : plan, section « Tranche 7 ».
4. Étude de rentabilisation (open-core) à relancer, puis à publier en artifact pour l'utilisateur.

## Pièges
- **Ne pas pousser `phase/v0.1.2` avant les correctifs** : le dépôt est public, les tests rouges décrivent des attaques sans authentification.
- Le dépôt principal reste sur `phase/interface` ; registre et handoff s'écrivent par un worktree détaché, `set -e`, HEAD vérifié.
- PR #10 est un brouillon pour la CI seulement : ne pas la fusionner avant la tranche 10.
- Un agent a lancé `docker image prune -f` sans mandat : l'interdire dans chaque prompt qui touche Docker.
- Après un redémarrage : relancer Docker Desktop ; VM Lima `hk-ubuntu` et `hk-fedora` arrêtées (ne jamais les recréer).

## Pré-requis
skills : econo, codex | variables d'env : PATH (Homebrew), DOCKER_HOST | services : Docker Desktop, Lima, `gh` authentifié
