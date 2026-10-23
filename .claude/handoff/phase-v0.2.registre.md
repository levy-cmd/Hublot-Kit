
## Suivi d'exécution (session de coordination)
- **Spec et plan v0.2** : c5ab9c0, puis fa75747 et 023747d. Validés par le relecteur et par Codex après deux correctifs : sécurité du parseur de couches, snapshots, coûts, historique `|N`, PAX, propriété de `server.go` et `fserr.go`.
  - Décisions de l'utilisateur : Q55 (F8 à F10 reportées), Q56 (l'état réel de la lecture seule est affiché, ce n'est pas le défaut), Q57 (snapshot permis en lecture seule après confirmation du coût), Q59 (sans contenu), Q60 (labels en liste blanche) ; les autres questions suivent la recommandation.
- **Tranche 1 (coquille Graphite & Émeraude)** : 9660cbd (tests rouges), 56e2af7, bfe5efe (tests rouges), 809f624. Validée par le relecteur et par Codex ; `make test`, `e2e` et `make build` verts.
  - Correctif de revue : sous 90 rem, un panneau du bas ne se monte qu'à sa première ouverture. Sinon, un panneau caché lançait un `docker export` d'un conteneur arrêté : 4 Gio, 5 min, conteneur verrouillé.
  - Aussi : contrôle i18n étendu ; `connected` égal à `Ping` ; liste avec Ports et noms prioritaires ; commande limitée à 3 lignes ; hauteur des panneaux.
  - Ruling: le point de Codex (sous 90 rem, un panneau déjà ouvert reste monté quand on le cache, et son flux de logs continue) passe en tête de la tranche 2. Le flux de logs doit se couper quand son panneau est caché, pour libérer une des 8 places.
  - Minor reporté : le test i18n ne vérifie que le préfixe des clés en gabarit (`container.tab.${k}`).
  - Reste pour la clôture : refaire les captures `docs/images/` (README encore en v0.1) et aligner `V=0.1.0` dans la doc d'installation.
