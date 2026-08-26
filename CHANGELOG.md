# Changelog

Tous les changements notables de ce projet seront documentés dans ce fichier.

Le format est basé sur [Keep a Changelog](https://keepachangelog.com/fr/1.0.0/),
et ce projet adhère au [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Non publié]

### Ajouté
- **Sprint 6 - Production Ready** : `docs/DEPLOYMENT.md` (guide systemd/docker-compose/k8s), artefacts `deploy/` (unit systemd durcie, compose prod, manifests k8s), `docs/RUNBOOK.md` (dépannage, sauvegarde/restauration, rollback), seuil couverture ≥90% en CI (GitHub + Woodpecker)
- Licence propriétaire SMiDeN 2026 (`LICENSE`, badge + section README)
- **Sprint 5 - Architecture Plugin** : interfaces `SourceConnector`/`DestinationConnector`/`EventTransformer`, registry `internal/plugin` (Register/New/ListAll), CalDAV natif en plugin `caldav`/`ics`, 5 transformers (`filter-private`, `mask-private`, `prefix-uid` auto, `filter-category`, `prefix-summary`) + `Pipeline`
- Config par plugin : `type` + `transformers: [{type, options}]` par source/destination, defaults `caldav`, validation, migration legacy
- API `GET /api/plugins` (auth requise) listant tous les plugins avec `type/kind/name/description`
- UI : Type dropdown (depuis `/api/plugins`), transformers par source/destination (add/delete, type + options JSON), onglet Plugins dédié
- Tests : `internal/plugin/plugin_test.go` (7 tests), `internal/web` plugins endpoint, config avec transformers, intégration via `plugin.NewSource/Destination`

### Modifié
- `sync.Syncer` utilise `plugin.SourceConnector/DestinationConnector` + `Pipeline` par destination (auto `filter-private` + `prefix-uid`), plus de dépendance directe à `caldav.Client`
- `cmd/synccal/main.go` crée les connecteurs via `plugin.NewSource/Destination` depuis `config.Type`
- `internal/web/config_dto.go` : vues `type` + `transformers`, merge avec defaults `caldav`
- `internal/web/assets/app.js` : refonte config (Type + Transformers), fetchPlugins, renderTransformers, collectConfig avec transformers, onglet Plugins, `views` étendu
- `config.example.yaml` documente `type` et `transformers` avec exemples

### Corrigé
- `internal/web/server_test.go` : `TestConfigPutUpdatesAndRebuilds` type manquant → default `caldav` dans merge, nouvelle config initiale avec `Type`
- `tests/integration/synccal_test.go` : migration vers `plugin.NewSource/Destination` + `sync.New` avec interfaces plugin

### Précédent (2026-08-26) - Sprint 4 + UI séparée
- Config découplée `sources[]` (name/url) + `destinations[]` (name/url/source) avec blocs séparés et dropdown source
- Validation `destination.source` + migration legacy, sync boucle sur `destinations`
- UI : blocs Sources/Destinations séparés, Type dropdown, login fix (`btn-login`/`btn-logout`/`Enter`)
- Tests adaptés, Nextcloud installé pour sync initiale réussie (3 events)

---

## [0.4.0] - 2026-08-19

### Ajouté
- Graceful shutdown amélioré : attente de fin de sync, arrêt propre du serveur HTTP

---

## [0.3.0] - 2026-08-19

### Ajouté
- Tests d'intégration avec Testcontainers (Nextcloud → Nextcloud, ICS public → Nextcloud)
- Docker-compose de test, Makefile (build/test/lint/docker), jeu de données de test
- Tests unitaires du package retry

---

## [0.2.0] - 2026-08-19

### Ajouté
- Package retry : backoff exponentiel + jitter, respect du header `Retry-After`
- Détection de changements CTag / sync-token (RFC 6578) avec persistance en SQLite (`source_state`)
- Configuration `filter_private` : exclusion des événements `CLASS:PRIVATE` / `CLASS:CONFIDENTIAL`
- Retour à une API CalDAV avec gestion des statuts HTTP retryables (408, 429, 500, 502, 503, 504)

---

## [0.1.0] - 2026-08-19

### Ajouté
- MVP Core :
  - Config YAML + validation (sources, destinations, sync, logging)
  - Client CalDAV source : publique (GET .ics + ETag) et authentifiée (REPORT calendar-query)
  - Client CalDAV destination : création, mise à jour, suppression (PUT/DELETE)
  - Mapping UID source → destination en SQLite (hash de contenu)
  - Logique de sync créer/maj/supprimer (soft/hard delete)
  - Scheduler horaire (cron) avec lock anti-concurrence
  - Healthcheck HTTP (`/healthz`, `/readyz`)
  - Logs JSON structurés (zap)
  - Métriques Prometheus (duration, events synced, erreurs, last sync)
- Infrastructure :
  - Versioning auto-bumper `M.m.f` (scripts/bump_version.py) — `feat:` → minor, autres → patch, major manuel
  - CI/CD GitHub Actions : lint, tests, bump version, build/push Docker multi-arch (amd64/arm64) vers dockhand
  - CI/CD Gitea (Woodpecker) : lint, tests, build/push Docker (sans bump)
  - Dockerfile multi-stage (binaire Go statique, alpine, non-root)
  - Documentation projet (PROJET.md, TODO.md, ROADMAP.md, CHANGELOG.md)

### Supprimé
- Interface API REST précédente (sera refaite dans le cadre de l'interface web)

---

## Convention de versioning (M.m.f)

| Type de commit | Bump | Exemple |
|----------------|------|---------|
| `feat:` | **m** (minor) | `feat: add multi-destination support` |
| `fix:` | **f** (patch) | `fix: handle timezone conversion bug` |
| `chore:`, `docs:`, `refactor:`, `test:`, `ci:`, `build:` | **f** (patch) | `chore: update dependencies` |
| **M** (major) | **Manuel uniquement** | Tag explicite `v1.0.0` |

### Règles
1. Les commits `feat:` incrémentent **m** (remise à zéro de **f**)
2. Tous les autres commits conventionnels incrémentent **f**
3. **M** ne change **jamais** automatiquement - tag manuel requis
4. Format de tag : `vM.m.f` (ex: `v0.1.0`, `v0.2.3`, `v1.0.0`)
