# Runbook opérationnel — SyncCal

Procédures d'exploitation : diagnostic, incidents, sauvegarde/restauration, rollback.
Public : exploitant SMiDeN. Pré-requis : accès SSH au serveur (ou kubectl) et au token web.

---

## 1. Santé du service

| Vérification | Commande | Attendu |
|---|---|---|
| Process | `systemctl status synccal` / `docker compose ps` / `kubectl -n synccal get pods` | active (running) / Up (healthy) / Running |
| Liveness | `curl -s http://localhost:8080/healthz` | `ok` |
| Readiness | `curl -s http://localhost:8080/readyz` | `ready` |
| Métriques | `curl -s http://localhost:8080/metrics \| grep synccal_` | compteurs présents |
| Logs | `journalctl -u synccal -f` / `docker logs -f synccal` / `kubectl logs -f` | JSON, niveau info |

Interprétation des endpoints :

* `/healthz` → **503 "sync in progress"** : normal pendant une sync ; anormal si > timeout configuré (`sync.timeout`).
* `/readyz` → **503 "db unavailable"** : problème SQLite (disque plein, permissions).

---

## 2. Incidents fréquents

### 2.1 Aucun événement synchronisé (compteurs à 0)

1. Dashboard → colonne *Statut* de la connexion : badge rouge = `last_error`.
2. Logs : chercher `"Connection sync failed"` ou `"Fetched source events"`.
3. Causes courantes :
   * Source injoignable → `curl -v <url_source>` depuis le serveur ;
   * Auth destination refusée (token expiré) → régénérer l'app password Nextcloud/Carbonio, mettre à jour via l'UI (Configuration) ;
   * URL destination incorrecte → doit pointer sur le calendrier CalDAV (`/remote.php/dav/calendars/<user>/<cal>/`).
4. Relancer : bouton *Synchroniser maintenant* (ou `POST /api/sync` avec le token).

### 2.2 HTTP 302 / redirection sur la destination

Le client ne suit pas les redirects volontairement. Un 302 signifie généralement :
Nextcloud non installé (page setup), session requise, ou mauvais chemin DAV.
Vérifier `https://<host>/status.php` puis l'URL exacte du calendrier.

### 2.3 Événements privés visibles dans la destination

`filter_private` désactivé OU transformer `mask-private` utilisé (masque mais ne supprime pas).
Correction : UI Configuration → cocher *Filtrer PRIVATE/CONFIDENTIAL* → Enregistrer.

### 2.4 Doublons d'UID entre sources

Les UIDs sont préfixés par hash source (`sha256(url)[:8]`). Des doublons indiquent
deux destinations pointant vers le même calendrier avec des sources différentes et un
mapping résiduel — voir §2.5.

### 2.5 Base SQLite corrompue

Symptôme : `/readyz` → 503 db unavailable, logs `database is locked` / `malformed`.

```bash
systemctl stop synccal                       # arrêter le process
cp /var/lib/synccal/data/synccal.db /tmp/synccal.db.corrupt
sqlite3 /var/lib/synccal/data/synccal.db "PRAGMA integrity_check;"
# Si corrompue : restaurer une sauvegarde (§4) — la resync est idempotente,
# les events sont recréés/mis à jour depuis les sources au prochain cycle.
systemctl start synccal
```

Note : en cas de perte totale de la DB, SyncCal reconstruit les mappings au premier run ;
les événements déjà présents dans la destination seront re-créés si leurs UID diffèrent
(préfixe source) — purger alors la destination si nécessaire.

### 2.6 Service qui redémarre en boucle

```bash
journalctl -u synccal -n 100 --no-pager      # cause du crash
ls -lh /var/lib/synccal                      # disque plein ?
df -h /var/lib/synccal
```

Causes : config YAML invalide (`config.Validate` fatal au démarrage) → corriger puis restart.

---

## 3. Sauvegarde

À planifier quotidiennement (cron) :

```bash
#!/bin/sh
# backup-synccal.sh — sqlite online backup (cohérent même service actif)
sqlite3 /var/lib/synccal/data/synccal.db ".backup '/srv/backups/synccal-$(date +%F).db'"
find /srv/backups -name 'synccal-*.db' -mtime +30 -delete
```

Sauvegarder aussi :

* `/etc/synccal/config.yaml` (contient les tokens — stockage chiffré/restricted) ;
* `data/plugins/` si plugins uploadés via l'UI.

Kubernetes :

```bash
kubectl -n synccal exec deploy/synccal -- tar cf - /app/data | tar xf - -C ./backup-synccal
```

## 4. Restauration

```bash
systemctl stop synccal
cp /srv/backups/synccal-2026-08-26.db /var/lib/synccal/data/synccal.db
chown synccal:synccal /var/lib/synccal/data/synccal.db
systemctl start synccal
curl -s http://localhost:8080/readyz          # ready
```

La première sync post-restauration réaligne les destinations (idempotent).

---

## 5. Rollback

### Binaire systemd

```bash
sudo systemctl stop synccal
sudo cp /usr/local/bin/synccal.bak-<version> /usr/local/bin/synccal   # conserver l'ancien binaire lors des mises à jour
sudo systemctl start synccal
```

Version déployée : `journalctl -u synccal | grep "Starting SyncCal"` ou `./synccal --help`
(la version est injectée via ldflags `git describe`).

### Docker

```bash
docker compose -f docker-compose.prod.yml down
# tag précédent visible dans le registry Gitea (tag = sha court)
sed -i 's/:latest/:<sha8-précédent>/' docker-compose.prod.yml
docker compose -f docker-compose.prod.yml up -d
```

### Kubernetes

```bash
kubectl -n synccal rollout undo deployment/synccal
kubectl -n synccal rollout history deployment/synccal
```

**Compatibilité DB** : les rollbacks restent sûrs tant que le schéma SQLite
(`event_mapping`, `source_state`) n'a pas évolué — vérifier CHANGELOG avant downgrade majeur.

---

## 6. Contacts & escalade

| Niveau | Qui | Quand |
|---|---|---|
| 1 | Exploitant SMiDeN (ce runbook) | tout incident |
| 2 | Responsable applicatif | incident > 1 h ou perte de données |
| 3 | Éditeur (auteur du projet) | bug confirmé → issue GitHub + logs JSON |

Liens utiles : [DEPLOYMENT.md](DEPLOYMENT.md) · [README](../README.md) · [CHANGELOG](../CHANGELOG.md)
