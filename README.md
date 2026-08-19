# SyncCal

Synchroniseur de calendriers CalDAV unidirectionnel : **une ou plusieurs sources** → **une ou plusieurs destinations** (Nextcloud, Carbonio...).

## Fonctionnalités

- **Sources flexibles** : calendrier public (URL `.ics` sans auth) **ou** authentifié (token / mot de passe d'application)
- **Multi-destinations** : sync vers plusieurs calendriers en parallèle
- **Détection de changements efficace** : CTag / sync-token (RFC 6578) en priorité, repli ETag / hash de contenu
- **Filtrage des événements privés** : exclusion `CLASS:PRIVATE` / `CLASS:CONFIDENTIAL` (`filter_private`)
- **Robustesse** : retry backoff exponentiel + jitter, respect du header `Retry-After`
- **Sync horaire** : scheduler configurable, lock anti-concurrence
- **Observabilité** : logs JSON structurés, métriques Prometheus, healthcheck `/healthz` + `/readyz`
- **Déploiement** : binaire statique Go unique, Docker multi-arch (amd64/arm64)

## Démarrage rapide

### Prérequis
- Go 1.23+ (pour compiler depuis les sources)
- Docker (optionnel, pour l'image ou les tests d'intégration)

### Compilation

```bash
make build
./bin/synccal -config config.yaml
```

### Configuration

Copiez `config.example.yaml` vers `config.yaml` et adaptez :

```yaml
# Source : publique (sans auth) ...
source:
  url: "https://example.com/calendar.ics"

# ... ou authentifiée (token / app password)
# source:
#   url: "https://cloud.example.com/remote.php/dav/calendars/user/source/"
#   username: "user"
#   password: "app-password-or-token"

# Destinations (authentifiées)
destinations:
  - name: "nextcloud-personal"
    url: "https://cloud.example.com/remote.php/dav/calendars/user/personal/"
    username: "user"
    password: "app-password-or-token"

sync:
  interval: "1h"          # 30m, 1h, 2h... ("0" = manuel seulement)
  timeout: "5m"
  delete_mode: "soft"     # "soft" | "hard"
  filter_private: true    # exclut les événements PRIVATE/CONFIDENTIAL
```

> **Sécurité** : privilégiez toujours un **token d'application** plutôt qu'un mot de passe de compte (Nextcloud et Carbonio le supportent).

### Docker

```bash
docker build -t synccal .
docker run -d \
  -p 8080:8080 \
  -v "$PWD/config.yaml:/app/config.yaml:ro" \
  -v synccal-data:/app/data \
  synccal
```

## Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /healthz` | Liveness — `ok` (503 si sync en cours) |
| `GET /readyz` | Readiness — `ready` (503 si sync en cours ou DB indisponible) |
| `GET /metrics` | Métriques Prometheus |

## Métriques Prometheus

- `synccal_sync_duration_seconds` — durée des syncs (histogramme)
- `synccal_events_synced_total` — événements créés / mis à jour / supprimés
- `synccal_sync_errors_total` — erreurs de sync
- `synccal_last_sync_timestamp` — dernière sync réussie par destination

## Développement

```bash
make test              # tests unitaires
make test-integration  # tests d'intégration (Docker requis, Testcontainers)
make lint              # golangci-lint
make docker-run        # infra de test locale (2× Nextcloud + serveur ICS public)
```

### Architecture

```
cmd/synccal/           # point d'entrée
internal/
  config/              # chargement + validation config YAML
  caldav/              # client CalDAV (source + destination, CTag/sync-token, retry)
  storage/             # SQLite (mapping UID, état source)
  sync/                # logique de synchronisation + scheduler
  retry/               # backoff exponentiel + jitter + Retry-After
  metrics/             # métriques Prometheus
tests/integration/     # tests d'intégration Testcontainers
scripts/               # versioning auto-bumper
```

## Feuille de route

| Sprint | Contenu | Statut |
|--------|---------|--------|
| 1-2 | MVP core + robustesse (config, CalDAV, sync, retry, CTag) | ✅ |
| 3 | Interface web (Material Design, light only) | 📋 priorité |
| 4 | Multi-sources (base du projet) | 📋 |
| 5 | Architecture plugin (principe du projet) | 📋 |
| 6 | Production ready (doc déploiement, runbook) | 📋 |

Voir [TODO.md](TODO.md), [ROADMAP.md](ROADMAP.md) et [PROJET.md](PROJET.md).

## Versioning

SemVer `M.m.f`, géré automatiquement par les commits (voir [CHANGELOG.md](CHANGELOG.md)) :

- `feat:` → bump **minor** (`m`)
- autres commits → bump **patch** (`f`)
- **major** (`M`) → tag manuel uniquement

## Dépôts

- GitHub : `git@github.com:lamacheref/synccal.git` (CI/CD + bump de version)
- Gitea : `ssh://gitea@gitea.smiden.eu:2222/flamachere/synccal.git` (CI/CD build image)

## Licence

À définir.