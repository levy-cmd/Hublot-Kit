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
