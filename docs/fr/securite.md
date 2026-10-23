# Modèle de sécurité

## Hublot-Kit est root sur l'hôte

L'agent parle au socket Docker, qui donne l'accès root à l'hôte, et avec `--pid=host` il lit les fichiers de
n'importe quel conteneur par `/proc/<pid>/root`. Qui peut se connecter à Hublot-Kit, ou prendre la main sur l'agent,
est root sur le serveur. Les mesures ci-dessous limitent les erreurs et l'exposition ; elles ne protègent pas contre
un agent compromis.

Traitez le jeton d'accès comme le mot de passe root du serveur.

## Exposition

- L'agent écoute sur `127.0.0.1:7878` par défaut. Sur toute autre adresse sans TLS, il affiche
  `WARNING: listening on … without TLS` au démarrage (légitime dans un conteneur publié sur `127.0.0.1`, ou derrière un reverse proxy).
- Accédez-y par un tunnel SSH, ou par un reverse proxy HTTPS avec `--public-origin`
  ([installation](installation.md#derrière-un-reverse-proxy-https)).
- L'agent n'ouvre aucune connexion sortante : ni télémétrie, ni recherche de mise à jour.

## Connexion et session

- Le jeton fait 32 octets aléatoires, affichés une seule fois. Seule son empreinte SHA-256 est stockée
  (`token.sha256`), et la comparaison se fait en temps constant. `hublot reset-token` le remplace.
- 5 échecs de connexion par minute et par adresse IP, puis `rate_limited`. Chaque tentative est comptée avant la
  lecture de son jeton : une salve de requêtes n'en obtient pas plus. Les échecs sont écrits dans le journal d'audit,
  10 lignes par minute au plus par adresse (une adresse IPv6 compte pour tout son /64) ; au-delà, une ligne
  `suppressed` pour cette adresse, et la minute suivante une ligne compte les échecs qui n'ont pas été écrits.
- Une connexion doit envoyer son premier octet en 5 secondes, une requête être lue en 15 secondes (ses en-têtes en
  10), une édition ou un envoi, une fois connecté, en 10 minutes ; une connexion inactive est fermée au bout d'une
  minute. L'agent garde au plus 512 connexions ouvertes, 64 par adresse (`--max-conns-per-ip`). Un téléchargement ou
  un flux de logs dont le client ne lit plus pendant 30 secondes est arrêté.
- La session vit en mémoire pendant 12 heures. Son cookie est `HttpOnly`, `SameSite=Strict`, et `Secure` avec TLS ou
  une origine publique en `https`.
- Toute requête autre que GET ou HEAD doit porter un en-tête `Origin` égal à l'origine de confiance
  (`--public-origin`, ou le `Host` de la requête). Les en-têtes `X-Forwarded-*` ne sont jamais crus.
- Rebinding DNS : sans `--public-origin`, l'origine de confiance vient de `Host`, donc un nom rebindé passe le
  contrôle d'`Origin` ; ce sont alors le cookie `SameSite=Strict` et le jeton qui vous protègent, et une page rebindée
  peut épuiser les tentatives de connexion de `127.0.0.1` pendant une minute. Exposé hors de `127.0.0.1`, renseignez
  `--public-origin`.

## Contenu des conteneurs

- Fichiers, logs, noms de fichiers et variables sont toujours échappés par l'interface ; `{@html}` est interdit dans
  le code, et une règle de lint le fait respecter. L'aperçu ne sert que du `text/plain` ou une image PNG, JPEG, GIF ou
  WebP, avec `nosniff` ; SVG et HTML s'affichent comme du texte, jamais rendus. Les téléchargements sont toujours en
  `attachment`.
- En-têtes : une `Content-Security-Policy` stricte (scripts limités à l'agent et aux empreintes de l'`index.html`
  embarqué, `frame-ancestors 'none'`), `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`.

## Lecture des fichiers

- L'agent épingle le process du conteneur par un pidfd, contrôle son cgroup, et ouvre chaque chemin avec
  `filepath-securejoin` dans la racine du conteneur : un lien symbolique ne peut jamais mener hors du conteneur.
- Les pseudo-systèmes de fichiers (`/proc`, `/sys` et assimilés) sont listés mais ne s'ouvrent pas : ils seraient lus
  avec les privilèges de l'agent sur l'hôte. Les nœuds de périphérique, FIFO et sockets sont refusés : le root d'un
  conteneur pourrait créer un nœud d'un périphérique de l'hôte.
- Un conteneur qui monte l'hôte (`-v /:/host`) montre l'hôte dans l'explorateur, et ses fichiers se lisent :
  l'utilisateur est root de toute façon. Le téléchargement d'un dossier d'un tel conteneur est refusé.

## Écriture des fichiers

- `--read-only` désactive toute écriture, en un seul endroit du code.
- Chaque écriture passe par une confirmation qui rappelle le conteneur et le chemin, puis par Docker
  (`PUT /archive`), jamais directement. Le chemin ne doit contenir aucun lien symbolique.
- L'écriture est refusée dans un conteneur qui monte `/`, `/proc`, `/sys` (ou dessous) ou `/dev` de l'hôte, et là où
  Docker n'écrit pas ce que voit le conteneur (`/etc/hosts`, tmpfs, points de montage).
- Le journal d'audit note l'intention avant l'écriture et le résultat après. Si l'intention ne s'écrit pas, rien
  n'est écrit dans le conteneur.

## Secrets

- La radiographie masque une variable quand son nom contient `PASSWORD`, `PASSWD`, `PASS`, `TOKEN`, `KEY`, `SECRET`,
  `CREDENTIAL`, `PRIVATE` ou `DSN`, quand sa valeur contient une URL avec identifiants, ou un mot de passe sous une
  forme connue (chaînes de connexion, `--requirepass …`, `Authorization: Bearer …`). La commande et le message
  d'erreur sont masqués partie par partie.
- Une valeur masquée se révèle une à une, et chaque révélation est écrite dans le journal d'audit (le nom, jamais la
  valeur) avant l'envoi de la valeur.
- C'est une heuristique : [certaines formes ne sont pas reconnues](limites.md#masquage-des-secrets). Les logs ne sont
  jamais masqués.

## Journal d'audit

`audit.log` dans le dossier de données, une ligne JSON par événement : connexions échouées, variables révélées,
fichiers édités et envoyés. Une connexion échouée note aussi l'adresse IP de la connexion TCP (jamais
`X-Forwarded-For`). Il est en ajout seul et n'est pas tourné en v0.1.

## Socket Docker

Monter le socket en `:ro` ne protège rien : c'est un socket, pas un fichier. Un proxy de socket Docker qui n'autorise
que les points d'API utilisés par Hublot-Kit (`_ping` ; conteneurs : liste, inspection, changements, logs, archive ;
volumes : inspection) réduit ce que l'agent peut demander à Docker, mais avec `--pid=host` l'agent reste root sur
l'hôte.

## Signaler une vulnérabilité

Merci d'utiliser le signalement privé de vulnérabilités de GitHub (onglet **Security** du dépôt, **Report a
vulnerability**) plutôt qu'un ticket public.
