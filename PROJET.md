# SyncCal - Synchroniseur de calendrier par caldav

## Objectif

Synchroniser un calendrier vers un ou plusieurs autres dans nextcloud et carbonio.

## Obligation

- La synchronisation se fait, suivant les possibilités, heure par heure, ou au changement si c'est détectable sans surcharge du serveur,
- Le calendrier de départ peut être public sans authentification OU un calendrier authentifié (login/mot de passe ou token),
- Le calendrier d'arrivée peut se trouver sur un compte avec login et mot de passe, 
- On préférera toujours un token à un mot de passe de compte (carbonio ou nextcloud le font.)
- Une interface graphique web, simple, Material Design clair (pas de mode sombre) pour gérer le produit — **configuration en blocs séparés Sources (nommées) / Destinations, avec liste déroulante source dans chaque destination**.
- Filtrage des événements marqués "PRIVATE" / "CONFIDENTIAL" (exclusion ou masquage détails)
- Architecture plugin pour les connecteurs (source/destination) et transformateurs d'événements

## Analyse

### Résumé du projet
Synchronisation de calendrier CalDAV unidirectionnelle (sources publiques **ou authentifiées** → destinations authentifiées) entre Nextcloud et Carbonio. **Modèle de config séparé** : `sources[]` (name/url) et `destinations[]` (name/url/source) découplés, chaque destination référence sa source par nom (dropdown UI).

### Faisabilité Technique

#### ✅ Points forts
- **CalDAV standard** : Protocole bien documenté, bibliothèques existantes (Python: caldav, icalendar; Go: caldav-go; Node: caldav)
- **Source flexible** : Support public (GET .ics) **et** authentifié (CalDAV REPORT calendar-query avec auth)
- **Destinations avec tokens** : Nextcloud et Carbonio supportent les app passwords / tokens d'application
- **Sync horaire** : Cron job ou systemd timer trivial à mettre en place
- **Détection de changements** : CTag / sync-token (RFC 6578), ETag/Last-Modified, ou comparaison hash du contenu .ics

#### ⚠️ Points d'attention
| Risque | Mitigation |
|--------|------------|
| Conflits d'UID (mêmes événements sur sources différentes) | Préfixer UID avec hash de la source (`sha256(url)[:8]`) |
| Suppression d'événements source | Logique de "soft delete" ou recréation complète |
| Rate limiting serveur | Backoff exponentiel, respect Retry-After |
| Volumes importants | Pagination CalDAV (REPORT calendar-query), streaming |
| Fuseaux horaires | Normaliser en UTC, conserver TZID original |
| Destinations référençant une source inexistante | Validation `destination.source ∈ sources[].name` + migration legacy `sources[].destination` → `destinations[]` |

#### 🔧 Choix techniques
- **Langage** : **Go** (décidé) — binaire statique unique (~10-15 Mo, pas de runtime), daemon long-running natif (scheduler, signaux, goroutines), concurrence pour le multi-destination (1 goroutine/dest), cross-compilation amd64/arm64 triviale, écosystème CalDAV mature (`emersion/go-ical`, `emersion/go-webdav/caldav`). Python était l'alternative (écosystème `caldav` mature, prototypage rapide) mais pénalisant pour le déploiement (runtime, image ~150 Mo).
- **Architecture** : Worker unique, stateless, config YAML (`sources` nommées + `destinations` avec `source`)
- **State** : Stockage local SQLite (mapping UID source → UID dest, hash contenu, état source CTag/sync-token keyé par URL)
- **Détection de changements** : CTag / sync-token (RFC 6578) prioritaire, repli ETag/hash si non supporté
- **Observabilité** : Logs structurés (JSON), métriques Prometheus, healthcheck HTTP (/healthz, /readyz)

### Estimation
- **MVP (1 source → 1 dest)** : ~2-3 semaines
- **Multi-destinations + tokens + robustesse** : +2 semaines
- **Tests d'intégration + CI/CD** : +1 semaine
- **Interface web Material Design** : +1 semaine (dont refactor blocs séparés + dropdown source)

### Verdict
**Faisable** - Projet bien délimité, techniquement standard, risque faible.
