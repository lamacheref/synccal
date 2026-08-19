# TODO - SyncCal

## Fait ✅
- [x] Parser config YAML + validation (URLs, destinations, filtres)
- [x] Client CalDAV lecture source publique (GET .ics + ETag) et authentifiée (REPORT calendar-query)
- [x] Client CalDAV écriture destination (auth token/basic, PUT/DELETE)
- [x] Mapping UID source → UID destination (SQLite + hash contenu)
- [x] Logique sync : créer, maj, supprimer (soft/hard delete)
- [x] Filtrage événements PRIVATE/CONFIDENTIAL (`filter_private`)
- [x] Détection de changements CTag / sync-token (RFC 6578) + ETag
- [x] Retry backoff exponentiel + jitter + respect Retry-After
- [x] Cron/Timer horaire + lock anti-concurrence
- [x] Healthcheck HTTP (/healthz, /readyz) + métriques Prometheus
- [x] Logs JSON structurés + graceful shutdown (SIGTERM, attente sync)
- [x] Tests unitaires (retry) + tests d'intégration (Testcontainers Nextcloud)
- [x] CI/CD GitHub (bump version) + Gitea (build image), multi-arch amd64/arm64
- [x] Dockerfile multi-stage, versioning auto-bumper M.m.f

## Sprint 3 - Production Ready
- [ ] Documentation déploiement (systemd, docker-compose, k8s)
- [ ] Runbook ops (dépannage, rollback)
- [ ] Interface web Material Design (light only) : dashboard, config, events, logs, déclenchement sync manuel

## Backlog (non priorisé)
- [ ] Support multi-sources (plusieurs calendriers publics)
- [ ] Notifications (email, webhook, ntfy) sur erreurs
- [ ] Sync bidirectionnelle optionnelle
- [ ] Filtres par catégorie / mots-clés
- [ ] Architecture plugin : connecteurs source/destination + transformateurs événements
- [ ] Support Carbonio dédié (tests d'intégration)
- [ ] Tests de charge / benchmark

## Bugs connus / Dette technique
- [ ] Gestion événements récurrents complexes (RRULE, EXDATE)
- [ ] Conflits UID si même événement dans 2 sources
- [ ] Fuseaux horaires : normaliser en UTC, conserver TZID original (partiel)
- [ ] Pagination CalDAV pour très gros volumes (REPORT streaming)