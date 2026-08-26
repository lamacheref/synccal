# Guide de déploiement — SyncCal

Production-ready : binaire statique Go, base SQLite locale, aucune dépendance runtime.

---

## Prérequis

1. **Binaire** : `make build` ou télécharger la release (`bin/synccal`, amd64/arm64).
2. **`config.yaml`** : copier `config.example.yaml` et renseigner sources/destinations + token web (voir README).
3. **Utilisateur système dédié** :
   ```bash
   sudo useradd --system --home /var/lib/synccal --shell /usr/sbin/nologin synccal
   sudo mkdir -p /etc/synccal /var/lib/synccal
   sudo cp config.yaml /etc/synccal/
   sudo chown -R synccal:synccal /etc/synccal /var/lib/synccal
   ```

> ⚠️ Le fichier config contient des mots de passe d'application : restreindre les droits
> `sudo chmod 640 /etc/synccal/config.yaml && sudo chgrp synccal /etc/synccal/config.yaml`.

---

## Option 1 — systemd (serveur nu)

```bash
sudo install -m 0755 bin/synccal /usr/local/bin/synccal
sudo cp deploy/systemd/synccal.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now synccal

# Vérifier
systemctl status synccal
curl -s http://localhost:8080/healthz     # ok
journalctl -u synccal -f                  # logs JSON
```

Mise à jour :

```bash
sudo systemctl stop synccal
sudo install -m 0755 bin/synccal.new /usr/local/bin/synccal
sudo systemctl start synccal
```

L'unité inclut durcissement (`ProtectSystem=strict`, `NoNewPrivileges`, etc.) et
arrêt propre : SIGTERM → SyncCal attend la fin de la sync en cours (max 40 s).

---

## Option 2 — Docker Compose

```bash
cd deploy/docker-compose
cp /chemin/vers/config.yaml .
docker compose -f docker-compose.prod.yml up -d
docker compose -f docker-compose.prod.yml ps        # healthy attendu
curl http://127.0.0.1:8080/healthz
```

* Port publié sur `127.0.0.1` uniquement — mettre un reverse proxy TLS devant (nginx/Caddy/Traefik).
* Volumes nommés `synccal-data` (SQLite) et `synccal-plugins` (archives plugins uploadées).
* Rotation de logs : json-file 10 Mo × 5.
* Mise à jour : `pull` + `up -d`.

### Reverse proxy nginx (exemple)

```nginx
server {
    listen 443 ssl;
    server_name synccal.example.com;
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

---

## Option 3 — Kubernetes

Manifests complets dans [`deploy/kubernetes/synccal.yaml`](../deploy/kubernetes/synccal.yaml)
(namespace, PVC 1 Gi, Deployment Recreate, Service, probes `/healthz`+`/readyz`, annotations Prometheus).

```bash
kubectl create namespace synccal
kubectl -n synccal create secret generic synccal-config --from-file=config.yaml
kubectl apply -f deploy/kubernetes/
kubectl -n synccal rollout status deploy/synccal
```

Points importants :

| Sujet | Choix |
|-------|-------|
| Répliques | **1** — SQLite n'est pas multi-écrivain ; `strategy: Recreate` impose l'exclusivité du PVC |
| Config | Secret `synccal-config` monté en lecture seule sur `/etc/synccal` |
| Persistance | PVC `synccal-data` (DB + état des sources + plugins uploadés) |
| Sécurité | runAsNonRoot 1000, readOnlyRootFilesystem, ALL capabilities dropped |
| Monitoring | annotations `prometheus.io/scrape` → `/metrics` |

Sauvegarde k8s : voir RUNBOOK (copie du contenu du PVC).

---

## Post-déploiement (toutes options)

1. **Vérifier** : `/healthz` → `ok`, `/readyz` → `ready`.
2. **UI** : se connecter avec `web.token`, onglet Dashboard → lancer *Synchroniser maintenant*.
3. **Supervision** : scraper `/metrics` (Prometheus), alerter sur
   `rate(synccal_sync_errors_total[15m]) > 0` et
   `time() - synccal_last_sync_timestamp > 7200` (aucune sync depuis 2 h).
4. **Sauvegardes** : planifier la sauvegarde de `data/synccal.db` (RUNBOOK §3).

---

## Matrice de décision

| Contexte | Recommandation |
|----------|----------------|
| 1 serveur, usage simple | systemd |
| Docker déjà présent | Compose (+ reverse proxy TLS) |
| Cluster k8s existant, supervision Prometheus | Kubernetes |
