# ROADMAP - SyncCal

## Vision
Devenir l'outil de référence pour la synchronisation CalDAV unidirectionnelle fiable, observable et zero-config vers Nextcloud et Carbonio.

---

## ✅ Fait - MVP Core + Robustesse (2026)
**Langage : Go** (binaire statique unique, daemon long-running, concurrence multi-dest)
- Config YAML + validation
- Client CalDAV source (publique GET .ics / authentifiée REPORT) + destination (PUT/DELETE)
- Mapping UID SQLite + hash contenu
- Sync créer/maj/supprimer (soft/hard) + lock anti-concurrence
- Filtrage événements PRIVATE/CONFIDENTIAL
- Détection de changements : CTag / sync-token + ETag
- Retry backoff + jitter + respect Retry-After
- Healthcheck /healthz /readyz + métriques Prometheus + logs JSON
- Graceful shutdown
- Tests unitaires + tests d'intégration Testcontainers (Nextcloud)
- CI/CD GitHub (bump) + Gitea (build), Docker multi-arch amd64/arm64
- Versioning auto-bumper M.m.f

---

## En cours - Interface web (Material Design, light only)
**Objectif** : Gestion du produit depuis une interface web simple
- Dashboard (statut sync, destinations, dernière sync)
- Configuration (source, destinations, intervalle, filtres)
- Visualisation des événements synchronisés
- Logs + déclenchement de sync manuel

---

## À venir - Production Ready
- Documentation déploiement (systemd, docker-compose, k8s)
- Runbook ops (dépannage, rollback)
- Support Carbonio dédié (tests d'intégration)

---

## Backlog - Idées futures
| Fonctionnalité | Priorité | Effort |
|----------------|----------|--------|
| Multi-sources (plusieurs calendriers) | Moyenne | Moyen |
| Notifications (email, webhook, ntfy) | Moyenne | Moyen |
| Sync bidirectionnelle | Basse | Élevé |
| Filtres avancés (catégories, regex) | Moyenne | Moyen |
| Architecture plugin (connecteurs, transformers) | Moyenne | Élevé |
| Support RRULE/EXDATE complet | Basse | Élevé |

---

## Principes de versioning
- **M** (Major) : Décision explicite utilisateur (breaking changes, v1.0)
- **m** (Minor) : Nouvelle fonctionnalité (feat:)
- **f** (Fix/Patch) : Tout autre commit (fix:, chore:, docs:, refactor:, test:, ci:, build:)

Format : `M.m.f` (ex: 0.1.0, 0.2.3, 1.0.0)