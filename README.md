<div align="center">

# 🔄 SyncCal

**Synchroniseur de calendriers CalDAV — Nextcloud & Carbonio**

[![Version](https://img.shields.io/badge/version-v0.1.0-blue?style=for-the-badge&logo=git&logoColor=white)](https://github.com/lamacheref/synccal/releases)
[![Langage](https://img.shields.io/badge/Go-1.23-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/licence-À_définir-808080?style=for-the-badge)]()

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
| **4** | Multi-sources (base du projet) | 📋 Planifié | ░░░░░░░░░░ 0% |
| **5** | Architecture plugin (principe du projet) | 📋 Planifié | ░░░░░░░░░░ 0% |
| **6** | Production ready (doc, runbook) | 📋 Planifié | ░░░░░░░░░░ 0% |

> **Progression globale :** `██████████ 50%` — 3/6 sprints terminés

## ✨ Fonctionnalités

- 🔀 **Sources flexibles** : calendrier public (URL `.ics`) **ou** authentifié (token / app password)
- 🎯 **Multi-destinations** : sync vers plusieurs calendriers en parallèle
- ⚡ **Détection de changements efficace** : CTag / sync-token (RFC 6578), repli ETag / hash
- 🔒 **Filtrage des événements privés** : exclusion `CLASS:PRIVATE` / `CLASS:CONFIDENTIAL`
- 🛡️ **Robustesse** : retry backoff exponentiel + jitter, respect du header `Retry-After`
- ⏰ **Sync horaire** : scheduler configurable, lock anti-concurrence
- 📊 **Observabilité** : logs JSON structurés, métriques Prometheus, `/healthz` + `/readyz`
- 🖥️ **Interface web** : dashboard, configuration, événements et logs dans le binaire (assets embarqués, Material Design light)
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

web:
  addr: "0.0.0.0:8080"    # interface web + API REST
  token: "change-me"      # token d'accès à l'UI et à l'API
```

> 🔐 **Sécurité** : privilégiez toujours un **token d'application** plutôt qu'un mot de passe de compte (Nextcloud et Carbonio le supportent).

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
| `/api/status` | GET | Statut de la sync (running, source, dernière sync, destinations) |
| `/api/config` | GET | Configuration actuelle (mots de passe masqués) |
| `/api/config` | PUT | Mise à jour de la configuration (recharge la sync) |
| `/api/events` | GET | Événements synchronisés (`?dest=<nom>` pour filtrer) |
| `/api/logs` | GET | Logs structurés capturés (`?level=<niveau>` pour filtrer) |
| `/api/sync` | POST | Déclenche une synchronisation immédiate |

### Métriques Prometheus

- `synccal_sync_duration_seconds` — durée des syncs (histogramme)
- `synccal_events_synced_total` — événements créés / mis à jour / supprimés
- `synccal_sync_errors_total` — erreurs de sync
- `synccal_last_sync_timestamp` — dernière sync réussie par destination

## 🛠️ Développement

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
  web/                 # interface web + API REST (assets embarqués via go:embed)
tests/integration/     # tests d'intégration Testcontainers
scripts/               # versioning auto-bumper
```

## 🗺️ Feuille de route

Voir [TODO.md](TODO.md), [ROADMAP.md](ROADMAP.md), [PROJET.md](PROJET.md) et [CHANGELOG.md](CHANGELOG.md).

## 📦 Dépôts

| Dépôt | URL | Rôle |
|-------|-----|------|
| **GitHub** | `git@github.com:lamacheref/synccal.git` | CI/CD + **bump de version** + releases |
| **Gitea** | `ssh://gitea@gitea.smiden.eu:2222/flamachere/synccal.git` | CI/CD build image Docker |

## 📄 Licence

À définir.