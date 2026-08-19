# Analyse de Faisabilité - SyncCal

## Résumé du projet
Synchronisation de calendrier CalDAV unidirectionnelle (source publique **ou authentifiée** → destinations authentifiées) entre Nextcloud et Carbonio.

## Faisabilité Technique

### ✅ Points forts
- **CalDAV standard** : Protocole bien documenté, bibliothèques existantes (Python: caldav, icalendar; Go: caldav-go; Node: caldav)
- **Source flexible** : Support public (GET .ics) **et** authentifié (CalDAV REPORT calendar-query avec auth)
- **Destinations avec tokens** : Nextcloud et Carbonio supportent les app passwords / tokens d'application
- **Sync horaire** : Cron job ou systemd timer trivial à mettre en place
- **Détection de changements** : ETag/Last-Modified sur le calendrier source, ou comparaison hash du contenu .ics

### ⚠️ Points d'attention
| Risque | Mitigation |
|--------|------------|
| Conflits d'UID (mêmes événements sur sources différentes) | Préfixer UID avec hash de la source |
| Suppression d'événements source | Logique de "soft delete" ou recréation complète |
| Rate limiting serveur | Backoff exponentiel, respect Retry-After |
| Volumes importants | Pagination CalDAV (REPORT calendar-query), streaming |
| Fuseaux horaires | Normaliser en UTC, conserver TZID original |

### 🔧 Choix techniques recommandés
- **Langage** : Go (binaire unique, perf, concurrency native) ou Python (écosystème CalDAV mature)
- **Architecture** : Worker unique, stateless, config YAML
- **State** : Stockage local SQLite (mapping UID source → UID dest, hash dernier sync)
- **Observabilité** : Logs structurés (JSON), métriques Prometheus, healthcheck HTTP

## Estimation
- **MVP (1 source → 1 dest)** : ~2-3 semaines
- **Multi-destinations + tokens + robustesse** : +2 semaines
- **Tests d'intégration + CI/CD** : +1 semaine

## Verdict
**Faisable** - Projet bien délimité, techniquement standard, risque faible.