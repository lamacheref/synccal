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
- [x] Correction du build cassé : go.mod réparé (go-ical pseudo-version, go-webdav v0.7.0, module Nextcloud supprimé), go.sum généré, client CalDAV réécrit en HTTP brut

## Sprint 3 - PRIORITÉ : Interface web (Material Design, light only)
- [x] **API REST backend** : endpoints status, config (sans mots de passe), events, logs, trigger sync
- [x] **Sécurité** : token d'accès à l'UI (config), pas d'exposition des mots de passe en clair
- [x] **Dashboard** : statut sync (source, intervalle, dernière sync), liste destinations + compteurs
- [x] **Configuration** : visualisation/édition source, destinations, intervalle, filtres
- [x] **Événements** : tableau des événements synchronisés (UID, hash, statut), filtre par destination
- [x] **Logs** : vue des logs structurés, filtre par niveau
- [x] **Actions** : bouton "Synchroniser maintenant"
- [x] **Assets embarqués** : fichiers statiques intégrés au binaire Go (embed), pas de CDN requis
- [x] **Tests UI** : tests des endpoints API + smoke test de la page
- [x] Tests d'intégration réparés et validés (Testcontainers Nextcloud + nginx, sync-token incrémental)

## Sprint 4 - Multi-sources (BASE du projet)
- [x] **Config 1:1** : `sources` liste de connexions, chaque source jumelée à SA destination (publique OU authentifiée par token / mot de passe d'application)
- [x] **Clients source/destination par connexion** : public (GET .ics + ETag) ou authentifié (REPORT calendar-query)
- [x] **État de sync par source** : CTag / sync-token stockés par source (table `source_state` keyée par URL)
- [x] **Préfixage UID par hash de source** : éviter les conflits si le même événement existe sur plusieurs sources (préfixe sha256(url)[:8], détection de suppression filtrée par source)
- [x] **Sync multi-connexions** : boucle sur toutes les connexions (1 source → 1 destination), une connexion en erreur ne bloque pas les autres
- [x] **Métriques par source** : duration, events, erreurs, last_sync par source
- [x] **Tests d'intégration multi-sources** : 2 sources (1 publique + 1 authentifiée) → 1 destination, UID partagé désambiguïsé, idempotence au 2e run

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