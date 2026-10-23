# Registre de la phase Interface (et correctif v0.1.1)

Demande de l'utilisateur (2026-10-23) : « un front à la hauteur du produit ». Rien qui ressemble à une interface générée par une IA, pas de thème indigo. Carte blanche sur la direction (« surprends-moi »). Le défaut des conteneurs arrêtés est corrigé en parallèle (v0.1.1).

## Parti pris visuel : « Quai »
- Univers : un terminal à conteneurs. Le hublot regarde dans les conteneurs d'un port.
- Couleurs (clair) : béton #E3E6E4, encre #1B2422, acier #6B7876, jaune de marquage #F2C230 (sélection, focus), vert signal #2E7D5B (en marche, sain), ambre #C98A1B (pause, redémarrage), rouille #9C3B26 (arrêté, erreur).
- Couleurs (sombre, « quai de nuit ») : asphalte #1E2624, texte #DDE3E0, même jaune ; états éclaircis pour garder le contraste AA.
- Typographie : Archivo (variable, axe de largeur : étroite pour titres et plaque, normale pour l'interface) ; JetBrains Mono SEULEMENT pour les contenus techniques (chemins, condensés, lignes de log, contenu de fichier). Polices auto-hébergées en woff2 (OFL, licence jointe), aucun CDN : l'interface est embarquée et doit marcher hors ligne.
- Signature, la seule audace : la **plaque de conteneur**, en tête de la page d'un conteneur, façon marquage ISO peint au pochoir sur une porte (identifiant court, image, état). Majuscules permises uniquement là, parce que c'est le vocabulaire du sujet.
- Lignes de tableau : liseré gauche à la couleur de l'état ; sélection en bande jaune « marquage au sol ».
- Interdits (les marques d'une interface générée) :
  - pas de kit de cartes arrondies ni d'ombres douces ; rayon de 2 px au plus ;
  - aucun dégradé ;
  - pas de surtitres en majuscules espacées ;
  - pas de « → » ajouté aux liens ;
  - pas de méta liées par « · » ;
  - pas d'animation d'entrée.
- Plancher de qualité : l'état est donné par le texte ET la couleur ; focus clavier visible (jaune) ; mouvement réduit respecté ; affichage jusqu'au mobile ; i18n FR/EN inchangée ; phrases à casse normale.

## Contrat des chantiers parallèles
- **Interface** (branche `phase/interface`) : possède `web/**` et le document de design. Ne touche à aucun fichier Go.
- **v0.1.1** (branche `phase/v0.1.1`) :
  - possède `internal/api/fs.go`, `internal/fsview/views.go`, le fichier d'erreurs de l'API (`internal/api/fserr.go` ou équivalent) et leurs tests ;
  - ne touche pas `web/` ;
  - ne change PAS le format JSON des réponses (codes, champs) : seul le texte du `detail` des 404 peut changer.
- Fichiers lus en commun : `web/src/lib/api.ts` (lu par v0.1.1), et le contrat d'API de la spec §6.
- Fusion par le coordinateur, chaque branche relue (relecteur + Codex) ; `make test` après fusion.

## Correctif v0.1.1 (branche `phase/v0.1.1`, PR brouillon #9)
- Commits : 2f32feb (tests rouges), 57798c8 (correction), 4f8d39d (test qui fige l'ordre des écritures). Validé par le relecteur ; Codex arbitré. CI verte.
- Cause, mesurée : dockerd verrouille le conteneur pendant tout `GET /archive`, donc l'`Inspect` fait au début de chaque relance attendait la fin de l'export et répondait 503 au bout de 10 s.
- Correction :
  - `Views.Pending` répond 202 sans interroger Docker, seulement pour une construction en cours, à égalité stricte d'ID complet ou de nom ;
  - `For` et `fsRequest` (GET et HEAD) le consultent avant l'`Inspect` ;
  - servir l'index exige toujours un nouvel `Inspect` (ID, `FinishedAt`, état arrêté, binds) ;
  - `SizeRootFs` est lu avant l'export, ce qui débloque la barre de progression ;
  - le `detail` des 404 est en langage d'utilisateur.
- Mesures : relances en 0,5 à 80 ms (contre 503 en 10 s) ; la progression avance.
- Ruling: écarté, le constat de Codex sur un nom réattribué qui recevrait un faux 202. dockerd verrouille rm et rename pendant l'export ; au pire, ce sont des 202 à tort pendant au plus 5 min, sans aucune donnée servie.
- Ruling: les écritures gardent l'`Inspect` en premier, conformément à l'ordre de §5.3. C'est figé par un test : 413, 403 et 400 pendant une indexation, la mutation donnant 202.
- Limites documentées : pendant un export, la radiographie, les logs et les écritures sur CE conteneur attendent la fin. Un F5 sur `/c/<id>` affiche « Docker ne répond pas » au bout de 10 s.
- Pistes non traitées : l'export à travers l'agent prend plus de 40 s, contre 11,6 s en direct ; `TestCost` (secrets) est instable sous charge sur hk-ubuntu.

## Interface, tranche 1 (c963718)
- Système « Quai », coquille, connexion, liste, plaque. `make test`, `make build` et e2e verts. Contrastes AA calculés.
- Écarts acceptés : couleurs « -texte » plus sombres pour le vert et l'ambre en clair (AA) ; liseré ambre à 2,34:1, l'état étant porté par le texte ; l'anneau du hublot, seule forme ronde.
- Retouches demandées pour la tranche 2 :
  - un jour entre les liserés de lignes voisines ;
  - la colonne « Statut » remplacée par une durée traduite (et le code de sortie quand il est anormal) ;
  - plus de contraste entre la porte de la plaque et le fond en sombre.
- Incident relevé par l'agent : vers 11 h, des conteneurs `hemera-*` ont été arrêtés par un `docker stop` normal. Ce n'était pas Hublot-Kit, qui était en lecture seule et n'a pas d'opération d'arrêt.
