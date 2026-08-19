# Changelog

Tous les changements notables de ce projet seront documentés dans ce fichier.

Le format est basé sur [Keep a Changelog](https://keepachangelog.com/fr/1.0.0/),
et ce projet adhère au [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Non publié]

### Ajouté
- Interface web Material Design (planifiée, non implémentée)

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
  - Config YAML + validation (source, destinations, sync, logging)
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