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

## Sprint 3 - PRIORITÉ : Interface web (Material Design, light only)
- [ ] **API REST backend** : endpoints status, config (sans mots de passe), events, logs, trigger sync
- [ ] **Sécurité** : token d'accès à l'UI (config), pas d'exposition des mots de passe en clair
- [ ] **Dashboard** : statut sync (source, intervalle, dernière sync), liste destinations + compteurs
- [ ] **Configuration** : visualisation/édition source, destinations, intervalle, filtres
- [ ] **Événements** : tableau des événements synchronisés (UID, hash, statut), filtre par destination
- [ ] **Logs** : vue des logs structurés, filtre par niveau
- [ ] **Actions** : bouton "Synchroniser maintenant"
- [ ] **Assets embarqués** : fichiers statiques intégrés au binaire Go (embed), pas de CDN requis
- [ ] **Tests UI** : tests des endpoints API + smoke test de la page

## Sprint 4 - Multi-sources (BASE du projet)
- [ ] **Config** : `source` devient une liste `sources` (chacune publique OU authentifiée par token / mot de passe d'application)
- [ ] **Clients source par source** : public (GET .ics + ETag) ou authentifié (REPORT calendar-query)
- [ ] **État de sync par source** : CTag / sync-token stockés par source (table `source_state` keyée par URL)
- [ ] **Préfixage UID par hash de source** : éviter les conflits si le même événement existe sur plusieurs sources
- [ ] **Sync multi-sources** : boucle sur toutes les sources → toutes les destinations
- [ ] **Métriques par source** : duration, events, erreurs, last_sync par source
- [ ] **Tests d'intégration multi-sources** : 2 sources (1 publique + 1 authentifiée) → 1 destination

## Sprint 5 - Architecture Plugin (PRINCIPE du projet)
- [ ] **Définir les interfaces Go** : `SourceConnector`, `DestinationConnector`, `EventTransformer`
- [ ] **Core CalDAV en plugin intégré** : Nextcloud/Carbonio comme connecteurs CalDAV par défaut
- [ ] **Registry plugins** : chargement/découverte des plugins (config par plugin, aucun changement core requis)
- [ ] **Event transformers** : pipeline de transformation (masquage détails PRIVATE, mapping catégories, renommage)
- [ ] **Config par plugin** : `plugin` dans la config YAML (type, options)
- [ ] **Interface web** : gestion des plugins dans l'UI (activation, options)
- [ ] **Tests plugins** : tests unitaires + intégration sur chaque connecteur/transformateur

## Sprint 6 - Production Ready
- [ ] Documentation déploiement (systemd, docker-compose, k8s)
- [ ] Runbook ops (dépannage, rollback)

## Backlog (non priorisé)
- [ ] Notifications (email, webhook, ntfy) sur erreurs
- [ ] Sync bidirectionnelle optionnelle
- [ ] Filtres par catégorie / mots-clés
- [ ] Support Carbonio dédié (tests d'intégration)
- [ ] Tests de charge / benchmark

## Bugs connus / Dette technique
- [ ] Gestion événements récurrents complexes (RRULE, EXDATE)
- [ ] Conflits UID si même événement dans 2 sources
- [ ] Fuseaux horaires : normaliser en UTC, conserver TZID original (partiel)
- [ ] Pagination CalDAV pour très gros volumes (REPORT streaming)