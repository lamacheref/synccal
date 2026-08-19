# ROADMAP - SyncCal

## Vision
Devenir l'outil de référence pour la synchronisation CalDAV unidirectionnelle fiable, observable et zero-config vers Nextcloud et Carbonio.

---

## v0.1.0 - MVP (Q3 2026)
**Objectif** : Sync basique 1 source publique → 1 destination authentifiée
- Config YAML minimaliste
- Sync horaire via cron
- Mapping UID SQLite
- Logs JSON + healthcheck
- Binaire unique Go

**Critères de sortie** : Sync stable pendant 1 semaine sur instance test

---

## v0.2.0 - Multi-dest & Robustesse (Q3 2026)
**Objectif** : Production-ready pour usage réel
- Multi-destinations par source
- Retry/backoff + rate limiting
- Métriques Prometheus
- Tests d'intégration (Testcontainers)
- Documentation déploiement

**Critères de sortie** : Déployé en prod sur 2+ instances réelles

---

## v0.3.0 - Observabilité & Ops (Q4 2026)
**Objectif** : Exploitabilité en environnement critique
- Dashboard Grafana fourni
- Alerting rules (PrometheusRule)
- Runbook complet
- Graceful shutdown / rolling update safe
- Multi-arch Docker (amd64/arm64)

---

## v1.0.0 - GA (Q1 2027)
**Objectif** : Version stable, API figée, support long terme
- SemVer strict (breaking = major)
- CHANGELOG complet
- Support RRULE/EXDATE complet
- Benchmarks publiés
- Migration guide v0.x → v1.0

---

## Post v1.0 - Idées futures
| Fonctionnalité | Priorité | Effort |
|----------------|----------|--------|
| Sync bidirectionnelle | Moyenne | Élevé |
| UI Web / API REST | Basse | Élevé |
| Filtres avancés (catégories, regex) | Moyenne | Moyen |
| Support CalDAV sharing (invites) | Basse | Élevé |
| Plugin system (transformers) | Basse | Élevé |

---

## Principes de versioning
- **M** (Major) : Décision explicite utilisateur (breaking changes, v1.0)
- **m** (Minor) : Nouvelle fonctionnalité (feat:)
- **f** (Fix/Patch) : Tout autre commit (fix:, chore:, docs:, refactor:, test:, ci:, build:)

Format : `M.m.f` (ex: 0.1.0, 0.2.3, 1.0.0)