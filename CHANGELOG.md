# Changelog

Tous les changements notables de ce projet seront documentés dans ce fichier.

Le format est basé sur [Keep a Changelog](https://keepachangelog.com/fr/1.0.0/),
et ce projet adhère au [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Non publié]

### Ajouté
- Analyse de faisabilité (FEASIBILITY.md)
- Backlog et planification (TODO.md)
- Feuille de route (ROADMAP.md)
- Configuration versioning auto-bumper M.m.f
- Pipeline CI/CD Docker pour dockhand

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

### Exemple de flux
```
v0.1.0 (tag initial)
  ├─ feat: add Prometheus metrics     → v0.2.0
  ├─ fix: retry logic on 429          → v0.2.1
  ├─ chore: update deps               → v0.2.2
  ├─ feat: multi-dest support         → v0.3.0
  └─ (user tags v1.0.0)               → v1.0.0
```