# ROADMAP - SyncCal

## Vision
Devenir l'outil de référence pour la synchronisation CalDAV unidirectionnelle fiable, observable et zero-config vers Nextcloud et Carbonio.

---

## ✅ Fait - MVP Core + Robustesse (2026)
**Langage : Go** (binaire statique unique, daemon long-running, concurrence multi-dest)
- Config YAML + validation (`sources`/`destinations` avec `type` + `transformers`, migration legacy)
- Client CalDAV source (publique GET .ics / authentifiée REPORT) + destination (PUT/DELETE) via plugin `caldav`
- Mapping UID SQLite + hash contenu
- Sync créer/maj/supprimer (soft/hard) + lock anti-concurrence (boucle sur `destinations`)
- Filtrage événements PRIVATE/CONFIDENTIAL via transformer
- Détection de changements : CTag / sync-token + ETag
- Retry backoff + jitter + respect Retry-After
- Healthcheck /healthz /readyz + métriques Prometheus + logs JSON
- Graceful shutdown
- Tests unitaires + tests d'intégration Testcontainers (Nextcloud) dont multi-sources
- CI/CD GitHub (bump) + Gitea (build), Docker multi-arch amd64/arm64
- Versioning auto-bumper M.m.f

---

## ✅ Fait - Interface web (Material Design, light only)
**Objectif** : Gestion du produit depuis une interface web simple — **terminé**
- Dashboard (statut sync, destinations avec `source_name`/`source_url`, dernière sync)
- Configuration en **blocs séparés** : Sources (name/type/url/user/pass + transformers) et Destinations (name/type/url/user/pass + dropdown source + transformers)
- Visualisation des événements synchronisés (filtre par destination)
- Logs + déclenchement de sync manuel
- Plugins : onglet dédié listant connecteurs et transformers via `/api/plugins`
- Assets embarqués `go:embed`, login `localStorage` + `Authorization: Bearer`

---

## ✅ Fait - Multi-sources / Destinations séparées (2026-08-26)
- Config découplée `sources[]` / `destinations[]` avec référence `destination.source`, validation et migration legacy
- Préfixage UID par hash de source (`prefix-uid` transformer automatique), état `source_state` par URL
- Sync multi-connexions : itération sur `destinations`, lookup source par nom, métriques par source/destination
- Tests d'intégration : 2 sources (public + authentifiée) → 2 destinations même URL, UID partagé désambiguïsé, idempotence

---

## ✅ Fait - Architecture Plugin (2026-08-26)
**Principe du projet — cœur extensible sans changement core**
- **Interfaces Go** : `SourceConnector`, `DestinationConnector`, `EventTransformer` (`internal/plugin/interfaces.go`)
- **Registry** : `RegisterSource/Destination/Transformer`, `NewSource/Destination/Transformer`, `ListAll` (thread-safe) — `internal/plugin/registry.go`
- **CalDAV natif en plugin** : `caldav` (source publique + auth) et `ics` alias, `caldav` destination — `internal/plugin/caldav.go`
- **Transformers** : `filter-private`, `mask-private`, `prefix-uid` (auto), `filter-category`, `prefix-summary` + `Pipeline` — `internal/plugin/transformer.go`
- **Config par plugin** : `type` + `transformers: [{type, options}]` par source/destination, defaults `caldav`, validation
- **Sync via plugins** : `sync.Syncer` utilise `plugin.SourceConnector/DestinationConnector` + `Pipeline` par destination (auto `filter-private` si `sync.filter_private` + `prefix-uid`)
- **Web UI** : Type dropdown (depuis `/api/plugins`), transformers add/delete avec options JSON, onglet Plugins
- **Tests** : `internal/plugin/plugin_test.go` (7 tests), `internal/web` plugins/config-with-plugins, intégration via `plugin.NewSource/Destination`

---

## ✅ Fait - Production Ready (2026-08-26)
- Documentation déploiement : `docs/DEPLOYMENT.md` (systemd durci, Compose prod avec healthcheck/rotation logs, k8s Recreate+PVC+probes+annotations Prometheus)
- Runbook ops : `docs/RUNBOOK.md` (santé, incidents 302/doublons/DB corrompue, sauvegarde/restauration SQLite, rollback binaire/Docker/k8s)
- CI : seuil couverture ≥90% enforced sur GitHub Actions et Woodpecker
- Reste au backlog : support Carbonio dédié (tests d'intégration)

---

## Backlog - Idées futures
| Fonctionnalité | Priorité | Effort |
|----------------|----------|--------|
| Notifications (email, webhook, ntfy) | Moyenne | Moyen |
| Sync bidirectionnelle | Basse | Élevé |
| Filtres avancés (catégories, regex) | Moyenne | Moyen |
| Support RRULE/EXDATE complet | Basse | Élevé |

---

## Principes de versioning
- **M** (Major) : Décision explicite utilisateur (breaking changes, v1.0)
- **m** (Minor) : Nouvelle fonctionnalité (feat:)
- **f** (Fix/Patch) : Tout autre commit (fix:, chore:, docs:, refactor:, test:, ci:, build:)

Format : `M.m.f` (ex: 0.1.0, 0.2.3, 1.0.0)
