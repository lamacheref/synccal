# TODO - SyncCal

## Backlog (non priorisé)
- [ ] Support multi-sources (plusieurs calendriers publics)
- [ ] Interface web de monitoring / dashboard
- [ ] Notifications (email, webhook, ntfy) sur erreurs
- [ ] Sync bidirectionnelle optionnelle
- [ ] Filtres par catégorie / mots-clés
- [ ] Tests de charge / benchmark

## Sprint 1 - MVP Core (Semaine 1-2)
- [ ] Parser config YAML (sources, destinations, scheduling)
- [ ] Client CalDAV lecture source publique (GET .ics + ETag)
- [ ] Client CalDAV écriture destination (auth token/basic)
- [ ] Mapping UID source → UID destination (SQLite)
- [ ] Logique sync : créer, maj, supprimer (soft delete)
- [ ] Gestion fuseaux horaires (UTC normalisé)
- [ ] Cron/Timer horaire + lock anti-concurrence
- [ ] Healthcheck HTTP (/healthz, /readyz)
- [ ] Logs JSON structurés + niveau configurable

## Sprint 2 - Robustesse & Multi-dest (Semaine 3-4)
- [ ] Retry avec backoff exponentiel + jitter
- [ ] Respect Retry-After / rate limiting
- [ ] Support multiples destinations par source
- [ ] Config validation au démarrage
- [ ] Tests d'intégration (Testcontainers Nextcloud/Carbonio)
- [ ] Métriques Prometheus (sync_duration, events_synced, errors_total)
- [ ] Graceful shutdown (SIGTERM handling)

## Sprint 3 - Production Ready (Semaine 5)
- [ ] Build multi-arch Docker (amd64, arm64)
- [ ] CI/CD pipeline complet
- [ ] Documentation déploiement (systemd, docker-compose, k8s)
- [ ] Runbook ops (dépannage, rollback)
- [ ] Versioning auto-bumper configuré

## Bugs connus / Dette technique
- [ ] Gestion événements récurrents complexes (RRULE, EXDATE)
- [ ] Conflits UID si même événement dans 2 sources