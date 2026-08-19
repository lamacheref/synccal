# SyncCal - Synchroniseur de calendrier par caldav

## Objectif

Synchroniser un calendrier vers un ou plusieurs autres dans nextcloud et carbonio.

## Obligation

- La synchronisation se fait, suivant les possibilités, heure par heure, ou au changement si c'est détectable sans surcharge du serveur,
- Le calendrier de départ peut être public sans authentification OU un calendrier authentifié (login/mot de passe ou token),
- Le calendrier d'arrivée peut se trouver sur un compte avec login et mot de passe, 
- On préférera toujours un token à un mot de passe de compte (carbonio ou nextcloud le font.)
- Une interface graphique web, simple, Material Design clair (pas de mode sombre) pour gérer le produit.

## Analyse

### Résumé du projet
Synchronisation de calendrier CalDAV unidirectionnelle (source publique **ou authentifiée** → destinations authentifiées) entre Nextcloud et Carbonio.

### Faisabilité Technique

#### ✅ Points forts
- **CalDAV standard** : Protocole bien documenté, bibliothèques existantes (Python: caldav, icalendar; Go: caldav-go; Node: caldav)
- **Source flexible** : Support public (GET .ics) **et** authentifié (CalDAV REPORT calendar-query avec auth)
- **Destinations avec tokens** : Nextcloud et Carbonio supportent les app passwords / tokens d'application
- **Sync horaire** : Cron job ou systemd timer trivial à mettre en place
- **Détection de changements** : ETag/Last-Modified sur le calendrier source, ou comparaison hash du contenu .ics

#### ⚠️ Points d'attention
| Risque | Mitigation |
|--------|------------|
| Conflits d'UID (mêmes événements sur sources différentes) | Préfixer UID avec hash de la source |
| Suppression d'événements source | Logique de "soft delete" ou recréation complète |
| Rate limiting serveur | Backoff exponentiel, respect Retry-After |
| Volumes importants | Pagination CalDAV (REPORT calendar-query), streaming |
| Fuseaux horaires | Normaliser en UTC, conserver TZID original |

#### 🔧 Choix techniques recommandés
- **Langage** : Go (binaire unique, perf, concurrency native) ou Python (écosystème CalDAV mature)
- **Architecture** : Worker unique, stateless, config YAML
- **State** : Stockage local SQLite (mapping UID source → UID dest, hash dernier sync)
- **Observabilité** : Logs structurés (JSON), métriques Prometheus, healthcheck HTTP

### Estimation
- **MVP (1 source → 1 dest)** : ~2-3 semaines
- **Multi-destinations + tokens + robustesse** : +2 semaines
- **Tests d'intégration + CI/CD** : +1 semaine
- **Interface web Material Design** : +1 semaine

### Verdict
**Faisable** - Projet bien délimité, techniquement standard, risque faible.
