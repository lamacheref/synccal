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
- [x] Correction login UI : listeners `btn-login`/`btn-logout`/`Enter` manquants (overlay `show`)

## Sprint 3 - Interface web (Material Design, light only) ✅
- [x] **API REST backend** : endpoints status, config (sans mots de passe), events, logs, trigger sync, plugins
- [x] **Sécurité** : token d'accès à l'UI (config), pas d'exposition des mots de passe en clair
- [x] **Dashboard** : statut sync (source, intervalle, dernière sync), liste destinations + compteurs (avec `source_name`)
- [x] **Configuration** : blocs séparés **Sources** (name/type/url/user/pass + transformers) et **Destinations** (name/type/url/user/pass + dropdown source + transformers) — Type dropdown depuis registry, transformers pipeline
- [x] **Événements** : tableau des événements synchronisés (UID, hash, statut), filtre par destination
- [x] **Logs** : vue des logs structurés, filtre par niveau
- [x] **Plugins** : onglet Plugins listant connecteurs et transformers (via `/api/plugins`)
- [x] **Actions** : bouton "Synchroniser maintenant"
- [x] **Assets embarqués** : fichiers statiques intégrés au binaire Go (embed), pas de CDN requis
- [x] **Tests UI** : tests des endpoints API + smoke test de la page + plugins endpoint
- [x] Tests d'intégration réparés et validés (Testcontainers Nextcloud + nginx, sync-token incrémental)

## Sprint 4 - Multi-sources / Destinations séparées (BASE du projet) ✅
- [x] **Config découplée** : `sources[]` (name/type/url/user/pass/transformers) + `destinations[]` (name/type/url/user/pass/source/transformers), validation `destination.source ∈ sources[].name`, migration legacy
- [x] **Clients source/destination par connexion** : public (GET .ics + ETag) ou authentifié (REPORT calendar-query), construits via `plugin.NewSource/Destination`
- [x] **État de sync par source** : CTag / sync-token stockés par source (table `source_state` keyée par URL)
- [x] **Préfixage UID par hash de source** : via transformer `prefix-uid` automatique, détection de suppression filtrée par source
- [x] **Sync multi-connexions** : boucle sur `destinations` (lookup `sourceIndex[source]`), une connexion en erreur ne bloque pas les autres, `connStats` keyé par `destination.name`
- [x] **Métriques par source/destination** : duration, events, erreurs, last_sync par source et destination
- [x] **Tests d'intégration multi-sources** : 2 sources (1 publique + 1 authentifiée) → 2 destinations même URL (`dest-public`/`dest-auth`), UID partagé désambiguïsé, idempotence au 2e run

## Sprint 5 - Architecture Plugin (PRINCIPE du projet) ✅
- [x] **Définir les interfaces Go** : `SourceConnector` (`HasChanged`/`Fetch`), `DestinationConnector` (`Create/Update/Delete/List`), `EventTransformer` (`Transform`)
- [x] **Core CalDAV en plugin intégré** : Nextcloud/Carbonio comme connecteurs `caldav`/`ics` par défaut via `internal/plugin/caldav.go` (wrap `caldav.Client`)
- [x] **Registry plugins** : `internal/plugin/registry.go` avec `RegisterSource/Destination/Transformer`, `NewSource/Destination/Transformer`, `ListAll` (thread-safe, trié) — aucun changement core requis pour ajouter un plugin
- [x] **Event transformers** : pipeline `internal/plugin/transformer.go` + `Pipeline` — `filter-private`, `mask-private`, `prefix-uid`, `filter-category`, `prefix-summary` (5 transformers, auto `filter-private` si `sync.filter_private` + `prefix-uid` par source)
- [x] **Config par plugin** : `type` + `transformers: [{type, options}]` dans `sources`/`destinations` (YAML `mapstructure`), validation et defaults `type=caldav`
- [x] **Interface web** : gestion des plugins dans l'UI — Type dropdown (depuis `/api/plugins`), transformers par source/destination (add/delete, type + options JSON), onglet Plugins listant tous les plugins avec description
- [x] **Tests plugins** : `internal/plugin/plugin_test.go` (7 tests : filter-private, mask-private, prefix, category, pipeline, registry, prefix hash) + `internal/web` plugins endpoint + config avec transformers

## Sprint 6 - Production Ready ✅
- [x] **Documentation déploiement** : `docs/DEPLOYMENT.md` + artefacts `deploy/` — unit systemd durcie (`deploy/systemd/synccal.service`), docker-compose prod avec healthcheck + rotation logs (`deploy/docker-compose/docker-compose.prod.yml`), manifests k8s namespace/PVC/Deployment Recreate/probes/Prometheus annotations (`deploy/kubernetes/synccal.yaml`)
- [x] **Runbook ops** : `docs/RUNBOOK.md` — santé, incidents fréquents (302, doublons UID, DB corrompue), sauvegarde sqlite `.backup`, restauration idempotente, rollback binaire/Docker/k8s
- [x] **CI seuil couverture** : enforcement ≥90% dans `.github/workflows/ci.yml` et `.woodpecker.yml`

## Backlog (non priorisé)
- [ ] Notifications (email, webhook, ntfy) sur erreurs
- [ ] Sync bidirectionnelle optionnelle
- [ ] Filtres par catégorie / mots-clés (partiellement via transformer `filter-category`)
- [ ] Support Carbonio dédié (tests d'intégration — nécessite image Carbonio conteneurisée)
- [ ] Tests de charge / benchmark

## Bugs connus / Dette technique
- [x] Fix login UI bloquant (2026-08-26) — listeners manquants ajoutés
- [ ] Gestion événements récurrents complexes (RRULE, EXDATE)
- [ ] Fuseaux horaires : normaliser en UTC, conserver TZID original (partiel)
- [ ] Pagination CalDAV pour très gros volumes (REPORT streaming)
