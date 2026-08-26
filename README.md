<div align="center">

# 🔄 SyncCal

**Synchroniseur de calendriers CalDAV — Nextcloud & Carbonio**

[![Version](https://img.shields.io/badge/version-v0.1.0-blue?style=for-the-badge&logo=git&logoColor=white)](https://github.com/lamacheref/synccal/releases)
[![Langage](https://img.shields.io/badge/Go-1.23-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/licence-Propriétaire_SMiDeN_2026-red?style=for-the-badge)](LICENSE)

[![Build GitHub](https://img.shields.io/badge/GitHub_CI-pending-yellow?style=flat-square&logo=github)](https://github.com/lamacheref/synccal/actions)
[![Build Gitea](https://img.shields.io/badge/Gitea_CI-pending-yellow?style=flat-square&logo=gitea)](https://gitea.smiden.eu/flamachere/synccal)
[![Multi-arch](https://img.shields.io/badge/amd64%20%7C%20arm64-supported-brightgreen?style=flat-square)]()

---

Synchronisez **un ou plusieurs calendriers sources** vers **une ou plusieurs destinations** :
calendrier public `.ics` ou authentifié (token / mot de passe d'application) → Nextcloud, Carbonio.

</div>

## 🏷️ Version

| | |
|---|---|
| **Version courante** | `v0.1.0` (MVP Core) |
| **Versioning** | SemVer `M.m.f` automatique via commits |
| **Tagging** | Auto sur GitHub (CI), mirroré sur Gitea |

## 📊 Progression des sprints

| Sprint | Contenu | Statut | Progression |
|--------|---------|--------|:-----------:|
| **1** | MVP Core — config, CalDAV, storage, sync, scheduler | ✅ Terminé | ██████████ 100% |
| **2** | Robustesse — retry, CTag/sync-token, tests, graceful shutdown | ✅ Terminé | ██████████ 100% |
| **3** | Interface web (Material Design, light only) | ✅ Terminé | ██████████ 100% |
| **4** | Multi-sources + destinations séparées (blocs nommés, dropdown) | ✅ Terminé | ██████████ 100% |
| **5** | Architecture plugin (connecteurs + transformers, upload archive) | ✅ Terminé | ██████████ 100% |
| **6** | Production ready (doc, runbook, CI seuil) | ✅ Terminé | ██████████ 100% |

> **Progression globale :** `██████████ 100%` — 6/6 sprints terminés

## ✨ Fonctionnalités

- 🔀 **Sources nommées + destinations séparées** : `sources[]` (`name/type/url`) et `destinations[]` (`name/type/url/source`) découplés, UID préfixés par hash (`sha256(url)[:8]`) via transformer `prefix-uid`
- 🎯 **Dropdown source** : chaque destination propose une liste déroulante de toutes les sources
- 🔌 **Architecture plugin** : connecteurs `SourceConnector`/`DestinationConnector` et `EventTransformer` via registry (`internal/plugin`), CalDAV natif en plugin `caldav`/`ics`, 5 transformers (`filter-private`, `mask-private`, `prefix-uid`, `filter-category`, `prefix-summary`) + `Pipeline`
- 📦 **Installation simple** : upload d'archive (`.zip`, `.tar.gz`) via UI Plugins → `POST /api/plugins/upload`, stockage `data/plugins`, liste `GET /api/plugins/installed`
- ⚡ **Détection de changements efficace** : CTag / sync-token (RFC 6578), repli ETag / hash
- 🔒 **Filtrage des événements privés** : exclusion `CLASS:PRIVATE`/`CLASS:CONFIDENTIAL` via transformer `filter-private` (auto si `sync.filter_private`)
- 🛡️ **Robustesse** : retry backoff exponentiel + jitter, respect du header `Retry-After`
- ⏰ **Sync horaire** : scheduler configurable, lock anti-concurrence, sync par destination
- 📊 **Observabilité** : logs JSON structurés, métriques Prometheus, `/healthz` + `/readyz`
- 🖥️ **Interface web** : Material Design light — **blocs titre** contenant des **blocs d'informations** (sections avec header coloré) sur toutes les pages, configuration Sources/Destinations avec Type + Transformers, événements, logs, et **Plugins** (upload + catalogue)
- 🐳 **Déploiement** : binaire statique Go unique, Docker multi-arch (amd64/arm64)

## 🚀 Démarrage rapide

### Prérequis
- Go 1.23+ (compilation depuis les sources)
- Docker (optionnel — image ou tests d'intégration)

### Compilation

```bash
make build
./bin/synccal -config config.yaml
```

### Configuration

Copiez `config.example.yaml` vers `config.yaml` et adaptez :

```yaml
# Sources : chaque source a un nom unique (référencé par les destinations)
sources:
  - name: "feries-fr"
    url: "https://example.com/calendar.ics"   # source publique (sans auth)
    # username: "user"                        # ou authentifiée
    # password: "app-password-or-token"

  # - name: "source-carbonio"
  #   url: "https://cloud.example.com/remote.php/dav/calendars/user/source/"
  #   username: "user"
  #   password: "app-password-or-token"

# Destinations : chaque destination référence une source via `source` (dropdown dans l'UI)
destinations:
  - name: "nextcloud-personal"
    url: "https://cloud.example.com/remote.php/dav/calendars/user/personal/"
    username: "user"
    password: "app-password-or-token"
    source: "feries-fr"

  # - name: "carbonio-work"
  #   url: "https://mail.example.com/dav/calendars/user/work/"
  #   username: "user@domain.com"
  #   password: "app-password"
  #   source: "source-carbonio"

sync:
  interval: "1h"          # 30m, 1h, 2h... ("0" = manuel seulement)
  timeout: "5m"
  delete_mode: "soft"     # "soft" | "hard"
  filter_private: true    # exclut les événements PRIVATE/CONFIDENTIAL

web:
  addr: "0.0.0.0:8080"    # interface web + API REST
  token: "change-me"      # token d'accès à l'UI et à l'API
```

> 🔐 **Sécurité** : privilégiez toujours un **token d'application** plutôt qu'un mot de passe de compte (Nextcloud et Carbonio le supportent).
> **Compatibilité** : l'ancien format `sources[].destination` est automatiquement migré vers `destinations[]` (avec `source: <nom>`).

### Variables d'environnement

En mode conteneur, deux mécanismes :

**1. Surcharge ponctuelle** — toute variable `SYNCCAL_<CHEMIN>` écrase la valeur du `config.yaml`
(champs en majuscules, `_` séparateur, index numériques pour les listes ; valeur vide = ignorée) :

```bash
SYNCCAL_WEB_TOKEN=mon-token          # web.token
SYNCCAL_SYNC_INTERVAL=30m            # sync.interval
SYNCCAL_SYNC_FILTER_PRIVATE=true     # sync.filter_private
SYNCCAL_DESTINATIONS_0_PASSWORD=x    # destinations[0].password
```

**2. Remplacement total** — `SYNCCAL_CONFIG` contient le YAML complet et remplace le fichier :

```yaml
# docker-compose.yml
environment:
  SYNCCAL_CONFIG: |
    sources:
      - name: feries
        type: caldav
        url: https://example.com/cal.ics
    destinations:
      - name: perso
        type: caldav
        url: https://cloud.example.com/remote.php/dav/calendars/user/personal/
        username: user
        password: app-password
        source: feries
    database:
      path: /app/data/synccal.db
    sync:
      interval: 1h
```

### Docker

```bash
docker build -t synccal .
docker run -d \
  -p 8080:8080 \
  -v "$PWD/config.yaml:/app/config.yaml:ro" \
  -v synccal-data:/app/data \
  synccal
```

## 🛰️ Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /healthz` | Liveness — `ok` (503 si sync en cours) |
| `GET /readyz` | Readiness — `ready` (503 si sync en cours ou DB indisponible) |
| `GET /metrics` | Métriques Prometheus |

### Interface web & API REST (section `web` de la config)

Tous les endpoints suivants sont protégés par le token (`Authorization: Bearer <token>` ou `X-API-Key: <token>`).

| Endpoint | Méthode | Description |
|----------|---------|-------------|
| `/` | GET | Application web (dashboard, config, événements, logs) |
| `/api/status` | GET | Statut de la sync (running, connexions, dernière sync) |
| `/api/config` | GET | Configuration actuelle (mots de passe masqués, `sources` + `destinations` séparés) |
| `/api/config` | PUT | Mise à jour de la configuration (recharge la sync) |
| `/api/events` | GET | Événements synchronisés (`?dest=<nom>` pour filtrer) |
| `/api/logs` | GET | Logs structurés capturés (`?level=<niveau>` pour filtrer) |
| `/api/sync` | POST | Déclenche une synchronisation immédiate |

### Métriques Prometheus

- `synccal_sync_duration_seconds` — durée des syncs par destination (histogramme)
- `synccal_events_synced_total` — événements créés / mis à jour / supprimés par destination
- `synccal_sync_errors_total` — erreurs de sync par destination
- `synccal_last_sync_timestamp` — dernière sync réussie par destination
- `synccal_source_events_total` — événements récupérés par source
- `synccal_source_errors_total` — erreurs par source
- `synccal_source_sync_duration_seconds` — durée de sync par source (histogramme)
- `synccal_source_last_sync_timestamp` — dernière sync réussie par source

## 🛠️ Développement

```bash
make test              # tests unitaires
make test-integration  # tests d'intégration (Docker requis, Testcontainers)
make lint              # golangci-lint
make docker-run        # infra de test locale (2× Nextcloud + serveur ICS public)
```

### Architecture

```
cmd/synccal/           # point d'entrée (plugin.NewSource/Destination)
internal/
  config/              # chargement + validation YAML (sources/destinations avec type + transformers)
  caldav/              # client CalDAV bas-niveau (utilisé par plugin caldav)
  plugin/              # registry + interfaces (SourceConnector/DestinationConnector/EventTransformer) + transformers (Pipeline) + caldav plugin
  storage/             # SQLite (mapping UID, état source)
  sync/                # logique de synchronisation via plugins + pipeline (boucle sur destinations)
  retry/               # backoff exponentiel + jitter + Retry-After
  metrics/             # métriques Prometheus
  web/                 # interface web + API REST (assets embarqués, sections titre, upload plugins)
tests/integration/     # tests d'intégration Testcontainers (via plugin)
scripts/               # versioning auto-bumper
```

## 🧪 Couverture de tests

**90.3% sur `internal/...`** — objectif ≥90% ✅ (8/8 packages testés, `go test` vert)

| Package | Couverture | Fichiers de tests |
|---------|:----------:|-------------------|
| `internal/caldav` | **89.2%** | `client_test.go` |
| `internal/config` | **95.1%** | `config_test.go` |
| `internal/metrics` | **100%** | `metrics_test.go` |
| `internal/plugin` | **95.1%** | `plugin_test.go` + `caldav_test.go` |
| `internal/retry` | **93.7%** | `retry_test.go` |
| `internal/storage` | **90.1%** | `storage_test.go` |
| `internal/sync` | **87.3%** | `syncer_test.go` |
| `internal/web` | **89.2%** | `server_test.go` + `logstore_test.go` |

### Serveur CalDAV mock

Les tests `caldav` et `plugin` s'appuient sur un mock `httptest.Server` simulant un serveur CalDAV minimal (réutilisable pour de futurs connecteurs, ex. Carbonio dédié) :

```
HEAD            → ETag (sources ICS publiques)
GET *.ics       → corps VCALENDAR + ETag
PUT             → 201 Created   (CreateEvent)
DELETE          → 204 NoContent (DeleteEvent)
PROPFIND D:0    → multistatus getctag + sync-token
PROPFIND D:1    → multistatus hrefs + getetags (listing ressources)
REPORT          → sync-collection : hrefs modifiés + nouveau sync-token (RFC 6578)
```

### Tests `sync` — mocks `SourceConnector` / `DestinationConnector`

* `New` : construction pipelines (auto `filter-private` + `prefix-uid`), référence source inconnue, transformer inconnu (warn + skip)
* `Sync` : création d'events avec UID préfixés, source inchangée → skip fetch, idempotence, update, filtrage PRIVATE/CONFIDENTIAL, erreurs source/destination non bloquantes, soft/hard delete, multi-destinations indépendantes
* Scheduler : interval=0 vs interval>0, pas de duplication au double `Start`
* Helpers : `sourcePrefix`, `hashContent`, conversions d'état, `parseEventsWithPipeline`

### Mesurer la couverture

```bash
go test -coverprofile=coverage.out -covermode=atomic ./internal/...
go tool cover -func=coverage.out | grep total   # total: (statements) 90.3%
go tool cover -html=coverage.out -o coverage.html
```

### Seuil CI recommandé

```yaml
- run: go test -coverprofile=coverage.out -covermode=atomic ./internal/...
- run: |
    COV=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | tr -d '%')
    awk "BEGIN {exit !($COV >= 90)}" || { echo "Coverage $COV% < 90%"; exit 1; }
```

## 🚢 Production

* Guide de déploiement : [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) — systemd durci, Docker Compose, Kubernetes
* Runbook exploitation : [docs/RUNBOOK.md](docs/RUNBOOK.md) — diagnostic, incidents, sauvegarde/restauration, rollback
* Artefacts prêts à l'emploi : [`deploy/`](deploy/) (systemd, docker-compose, kubernetes)

## 🗺️ Feuille de route

Voir [TODO.md](TODO.md), [ROADMAP.md](ROADMAP.md), [PROJET.md](PROJET.md) et [CHANGELOG.md](CHANGELOG.md).

## 📦 Dépôts

| Dépôt | URL | Rôle |
|-------|-----|------|
| **GitHub** | `git@github.com:lamacheref/synccal.git` | CI/CD + **bump de version** + releases + image `ghcr.io/lamacheref/synccal` |
| **Gitea** | `ssh://gitea@gitea.smiden.eu:2222/flamachere/synccal.git` | CI/CD + image `gitea.smiden.eu/flamachere/synccal` |

## 📄 Licence

**Propriétaire — Copyright © 2026 SMiDeN. Tous droits réservés.**

Logiciel développé par SMiDeN, destiné à un **usage unique et exclusif au sein de SMiDeN**.
Reproduction, modification, distribution ou exploitation hors du cadre de SMiDeN interdites sans autorisation écrite préalable.

Voir le fichier [LICENSE](LICENSE) pour les termes complets.
