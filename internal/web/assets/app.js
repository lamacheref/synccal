"use strict";

const $ = (sel) => document.querySelector(sel);
const $$ = (sel) => Array.from(document.querySelectorAll(sel));

let token = localStorage.getItem("synccal_token") || "";

async function api(path, opts = {}) {
  const headers = Object.assign({}, opts.headers || {});
  if (token) headers["Authorization"] = "Bearer " + token;
  if (opts.body) headers["Content-Type"] = "application/json";
  const res = await fetch(path, Object.assign({}, opts, { headers }));
  if (res.status === 401) {
    requireLogin("Token invalide ou expiré");
    throw new Error("unauthorized");
  }
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || res.status + " " + res.statusText);
  return data;
}

function requireLogin(message) {
  $("#btn-logout").classList.add("hidden");
  $("#login-screen").classList.add("show");
  $("#login-message").textContent = message || "";
  $("#login-message").classList.toggle("hidden", !message);
  $("#login-token").value = "";
  $("#login-token").focus();
}

function doLogin() {
  const t = $("#login-token").value.trim();
  if (!t) return;
  token = t;
  localStorage.setItem("synccal_token", t);
  $("#login-screen").classList.remove("show");
  $("#btn-logout").classList.remove("hidden");
  const active = $(".tab.active");
  switchTab(active ? active.dataset.tab : "dashboard");
}

function doLogout() {
  token = "";
  localStorage.removeItem("synccal_token");
  requireLogin("Déconnecté. Entrez à nouveau le token pour continuer.");
}

$("#btn-login").addEventListener("click", doLogin);
$("#login-token").addEventListener("keydown", (e) => {
  if (e.key === "Enter") doLogin();
});
$("#btn-logout").addEventListener("click", doLogout);

let snackTimer = null;
function snack(message, isError = false) {
  const el = $("#snackbar");
  el.textContent = message;
  el.className = "snackbar show" + (isError ? " error" : "");
  clearTimeout(snackTimer);
  snackTimer = setTimeout(() => (el.className = "snackbar"), 3500);
}

function fmtTime(ts) {
  if (!ts) return "—";
  const d = new Date(ts);
  if (isNaN(d.getTime())) return ts;
  return d.toLocaleString("fr-FR");
}

function esc(s) {
  return String(s == null ? "" : s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

// ---------------------------------------------------------------------------
// Tabs
// ---------------------------------------------------------------------------

const views = { dashboard: "#view-dashboard", config: "#view-config", events: "#view-events", logs: "#view-logs" };

function switchTab(name) {
  $$(".tab").forEach((t) => t.classList.toggle("active", t.dataset.tab === name));
  Object.entries(views).forEach(([key, sel]) => $(sel).classList.toggle("active", key === name));
  loadView(name);
}

function loadView(name) {
  if (name === "dashboard") loadDashboard();
  else if (name === "config") loadConfigForm();
  else if (name === "events") loadEvents();
  else if (name === "logs") loadLogs();
}

$$(".tab").forEach((t) => t.addEventListener("click", () => switchTab(t.dataset.tab)));

// ---------------------------------------------------------------------------
// Sync action
// ---------------------------------------------------------------------------

async function triggerSync() {
  const btn = $("#btn-sync");
  btn.disabled = true;
  try {
    await api("/api/sync", { method: "POST" });
    snack("Synchronisation lancée");
    $("#run-indicator").classList.remove("hidden");
    pollStatus();
  } catch (e) {
    snack("Échec de la synchronisation: " + e.message, true);
  } finally {
    btn.disabled = false;
  }
}

async function pollStatus() {
  try {
    const st = await api("/api/status");
    if (st.running) {
      $("#run-indicator").classList.remove("hidden");
      setTimeout(pollStatus, 1500);
    } else {
      $("#run-indicator").classList.add("hidden");
      loadDashboard();
    }
  } catch (e) {
    /* ignore transient errors */
  }
}

$("#btn-sync").addEventListener("click", triggerSync);

// ---------------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------------

async function loadDashboard() {
  let st;
  try {
    st = await api("/api/status");
  } catch (e) {
    $("#view-dashboard").innerHTML = `<div class="card"><div class="empty">Impossible de charger le statut : ${esc(e.message)}</div></div>`;
    return;
  }

  const lastSync = fmtTime(st.last_sync);
  const dur = st.last_duration_sec ? st.last_duration_sec.toFixed(1) + "s" : "—";
  const state = st.running ? "err" : st.last_error ? "err" : "ok";
  const stateLabel = st.running ? "En cours" : st.last_error ? "Erreur" : "OK";

  let destRows = "";
  if (st.destinations && st.destinations.length) {
    destRows = st.destinations
      .map(
        (d) => `
        <tr>
          <td>${esc(d.name)}</td>
          <td>${d.last_error ? `<span class="badge err">${esc(d.last_error)}</span>` : `<span class="badge ok">OK</span>`}</td>
          <td>${fmtTime(d.last_sync)}</td>
          <td>${d.created}</td>
          <td>${d.updated}</td>
          <td>${d.deleted}</td>
          <td>${d.errors}</td>
          <td>${d.last_duration_sec ? d.last_duration_sec.toFixed(1) + "s" : "—"}</td>
        </tr>`
      )
      .join("");
  } else {
    destRows = `<tr><td colspan="8" class="empty">Aucune destination configurée</td></tr>`;
  }

  $("#view-dashboard").innerHTML = `
    <div class="grid cols-3">
      <div class="card stat"><div class="value ${state}">${stateLabel}</div><div class="label">État de la synchronisation</div></div>
      <div class="card stat"><div class="value text">${esc(st.source_url)}</div><div class="label">Source</div></div>
      <div class="card stat"><div class="value">${esc(st.interval || "manuel")}</div><div class="label">Intervalle</div></div>
      <div class="card stat"><div class="value">${esc(lastSync)}</div><div class="label">Dernière synchronisation</div></div>
      <div class="card stat"><div class="value">${esc(dur)}</div><div class="label">Dernière durée</div></div>
      ${st.last_error ? `<div class="card stat"><div class="value err">${esc(st.last_error)}</div><div class="label">Dernière erreur</div></div>` : ""}
    </div>
    <div class="card">
      <h2>Destinations</h2>
      <div class="table-wrap">
        <table>
          <thead>
            <tr><th>Nom</th><th>Statut</th><th>Dernière sync</th><th>Créés</th><th>Mis à jour</th><th>Supprimés</th><th>Erreurs</th><th>Durée</th></tr>
          </thead>
          <tbody>${destRows}</tbody>
        </table>
      </div>
    </div>`;
}

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

let configCache = null;

async function loadConfigForm() {
  let cfg;
  try {
    cfg = await api("/api/config");
    configCache = cfg;
  } catch (e) {
    $("#view-config").innerHTML = `<div class="card"><div class="empty">Impossible de charger la configuration : ${esc(e.message)}</div></div>`;
    return;
  }

  const destFields = cfg.destinations
    .map(
      (d, i) => `
      <div class="card">
        <h2>Destination ${i + 1}</h2>
        <div class="form-grid">
          <div class="field"><label>Nom</label><input data-dest="${i}" data-field="name" value="${esc(d.name)}"></div>
          <div class="field"><label>URL</label><input data-dest="${i}" data-field="url" value="${esc(d.url)}"></div>
          <div class="field"><label>Utilisateur</label><input data-dest="${i}" data-field="username" value="${esc(d.username)}" autocomplete="off"></div>
          <div class="field"><label>Mot de passe / token <span class="hint">(laisser vide pour conserver)</span></label><input data-dest="${i}" data-field="password" type="password" autocomplete="new-password"></div>
        </div>
      </div>`
    )
    .join("");

  $("#view-config").innerHTML = `
    <form id="config-form">
      <div class="card">
        <h2>Source</h2>
        <div class="form-grid">
          <div class="field"><label>URL</label><input id="cfg-source-url" value="${esc(cfg.source.url)}"></div>
          <div class="field"><label>Utilisateur</label><input id="cfg-source-username" value="${esc(cfg.source.username)}" autocomplete="off"></div>
          <div class="field"><label>Mot de passe / token <span class="hint">(laisser vide pour conserver)</span></label><input id="cfg-source-password" type="password" autocomplete="new-password"></div>
        </div>
      </div>

      <div id="destinations">${destFields}</div>

      <div class="card">
        <h2>Synchronisation</h2>
        <div class="form-grid">
          <div class="field"><label>Intervalle</label><input id="cfg-sync-interval" value="${esc(cfg.sync.interval)}" placeholder="1h, 30m, 0 (manuel)"></div>
          <div class="field"><label>Timeout</label><input id="cfg-sync-timeout" value="${esc(cfg.sync.timeout)}"></div>
          <div class="field"><label>Batch size</label><input id="cfg-sync-batch" type="number" value="${cfg.sync.batch_size}"></div>
          <div class="field"><label>Mode de suppression</label>
            <select id="cfg-sync-delete">
              <option value="soft" ${cfg.sync.delete_mode === "soft" ? "selected" : ""}>Soft (marquer supprimé)</option>
              <option value="hard" ${cfg.sync.delete_mode === "hard" ? "selected" : ""}>Hard (purge)</option>
            </select>
          </div>
          <div class="field check"><input id="cfg-sync-private" type="checkbox" ${cfg.sync.filter_private ? "checked" : ""}><label for="cfg-sync-private">Filtrer les événements PRIVATE / CONFIDENTIAL</label></div>
        </div>
      </div>

      <div class="card">
        <h2>Interface web</h2>
        <div class="form-grid">
          <div class="field"><label>Adresse d'écoute</label><input id="cfg-web-addr" value="${esc(cfg.web.addr)}"></div>
          <div class="field"><label>Token d'accès <span class="hint">(${cfg.web.token_set ? "défini" : "non défini — accès libre"})</span></label><input id="cfg-web-token" type="password" placeholder="${cfg.web.token_set ? "•••••••• (laisser vide pour conserver)" : ""}"></div>
        </div>
      </div>

      <div class="card">
        <h2>Journalisation</h2>
        <div class="form-grid">
          <div class="field"><label>Niveau</label>
            <select id="cfg-log-level">
              ${["debug", "info", "warn", "error"].map((l) => `<option value="${l}" ${cfg.logging.level === l ? "selected" : ""}>${l}</option>`).join("")}
            </select>
          </div>
          <div class="field"><label>Format</label>
            <select id="cfg-log-format">
              ${["json", "console"].map((f) => `<option value="${f}" ${cfg.logging.format === f ? "selected" : ""}>${f}</option>`).join("")}
            </select>
          </div>
        </div>
      </div>

      <div class="toolbar">
        <button type="submit" class="btn btn-primary">Enregistrer la configuration</button>
        <button type="button" class="btn btn-outline" id="btn-add-dest">+ Ajouter une destination</button>
      </div>
    </form>`;

  $("#btn-add-dest").addEventListener("click", () => addDestinationCard());
  $("#config-form").addEventListener("submit", saveConfig);
}

function addDestinationCard() {
  const wrap = $("#destinations");
  const i = wrap.querySelectorAll(".card").length;
  const card = document.createElement("div");
  card.className = "card";
  card.innerHTML = `
    <h2>Destination ${i + 1}</h2>
    <div class="form-grid">
      <div class="field"><label>Nom</label><input data-dest="${i}" data-field="name" value=""></div>
      <div class="field"><label>URL</label><input data-dest="${i}" data-field="url" value=""></div>
      <div class="field"><label>Utilisateur</label><input data-dest="${i}" data-field="username" value="" autocomplete="off"></div>
      <div class="field"><label>Mot de passe / token</label><input data-dest="${i}" data-field="password" type="password" autocomplete="new-password"></div>
    </div>`;
  wrap.appendChild(card);
}

function collectConfig() {
  const cfg = JSON.parse(JSON.stringify(configCache || {}));
  cfg.source = {
    url: $("#cfg-source-url").value,
    username: $("#cfg-source-username").value,
  };
  const srcPass = $("#cfg-source-password").value;
  if (srcPass) cfg.source.password = srcPass;

  cfg.destinations = $$("#destinations .card").map((card) => {
    const d = { name: "", url: "", username: "" };
    card.querySelectorAll("[data-field]").forEach((input) => {
      const key = input.dataset.field;
      if (key === "password") {
        if (input.value) d.password = input.value;
      } else {
        d[key] = input.value;
      }
    });
    return d;
  });

  cfg.sync = {
    interval: $("#cfg-sync-interval").value,
    timeout: $("#cfg-sync-timeout").value,
    batch_size: parseInt($("#cfg-sync-batch").value, 10) || 100,
    delete_mode: $("#cfg-sync-delete").value,
    filter_private: $("#cfg-sync-private").checked,
  };
  cfg.web = { addr: $("#cfg-web-addr").value };
  const tok = $("#cfg-web-token").value;
  if (tok) cfg.web.token = tok;
  cfg.logging = {
    level: $("#cfg-log-level").value,
    format: $("#cfg-log-format").value,
  };
  delete cfg.database;
  return cfg;
}

async function saveConfig(e) {
  e.preventDefault();
  const btn = e.target.querySelector('[type="submit"]');
  btn.disabled = true;
  try {
    const cfg = collectConfig();
    const updated = await api("/api/config", { method: "PUT", body: JSON.stringify(cfg) });
    configCache = updated;
    snack("Configuration enregistrée");
    loadDashboard();
  } catch (err) {
    snack("Erreur: " + err.message, true);
  } finally {
    btn.disabled = false;
  }
}

// ---------------------------------------------------------------------------
// Events
// ---------------------------------------------------------------------------

const eventsState = { dest: "", offset: 0, limit: 50 };

async function loadEvents() {
  let cfg = configCache;
  if (!cfg) {
    try {
      cfg = await api("/api/config");
      configCache = cfg;
    } catch (e) { /* ignore */ }
  }

  const destOptions = (cfg ? cfg.destinations : [])
    .map((d) => `<option value="${esc(d.name)}" ${eventsState.dest === d.name ? "selected" : ""}>${esc(d.name)}</option>`)
    .join("");

  let data;
  try {
    const q = new URLSearchParams({ limit: String(eventsState.limit), offset: String(eventsState.offset) });
    if (eventsState.dest) q.set("destination", eventsState.dest);
    data = await api("/api/events?" + q.toString());
  } catch (e) {
    $("#view-events").innerHTML = `<div class="card"><div class="empty">Impossible de charger les événements : ${esc(e.message)}</div></div>`;
    return;
  }

  const rows = (data.events || [])
    .map(
      (ev) => `
      <tr>
        <td class="mono">${esc(ev.source_uid)}</td>
        <td>${esc(ev.dest_name)}</td>
        <td class="hash mono" title="${esc(ev.content_hash)}">${esc(ev.content_hash)}</td>
        <td>${ev.deleted ? '<span class="badge deleted">supprimé</span>' : '<span class="badge ok">synchronisé</span>'}</td>
        <td>${fmtTime(ev.synced_at)}</td>
      </tr>`
    )
    .join("");

  const hasPrev = eventsState.offset > 0;
  const hasNext = (data.events || []).length >= eventsState.limit;

  $("#view-events").innerHTML = `
    <div class="card">
      <div class="toolbar">
        <select id="ev-filter" class="field">
          <option value="">Toutes les destinations</option>
          ${destOptions}
        </select>
        <span class="spacer"></span>
        <div class="pager">
          <button class="btn btn-outline btn-small" id="ev-prev" ${hasPrev ? "" : "disabled"}>Précédent</button>
          <span>offset ${eventsState.offset}</span>
          <button class="btn btn-outline btn-small" id="ev-next" ${hasNext ? "" : "disabled"}>Suivant</button>
        </div>
      </div>
      <div class="table-wrap">
        <table>
          <thead>
            <tr><th>UID source</th><th>Destination</th><th>Hash contenu</th><th>Statut</th><th>Synchronisé le</th></tr>
          </thead>
          <tbody>${rows || `<tr><td colspan="5" class="empty">Aucun événement</td></tr>`}</tbody>
        </table>
      </div>
    </div>`;

  $("#ev-filter").addEventListener("change", (e) => {
    eventsState.dest = e.target.value;
    eventsState.offset = 0;
    loadEvents();
  });
  $("#ev-prev").addEventListener("click", () => {
    eventsState.offset = Math.max(0, eventsState.offset - eventsState.limit);
    loadEvents();
  });
  $("#ev-next").addEventListener("click", () => {
    eventsState.offset += eventsState.limit;
    loadEvents();
  });
}

// ---------------------------------------------------------------------------
// Logs
// ---------------------------------------------------------------------------

const logsState = { level: "", limit: 200 };

async function loadLogs() {
  const q = new URLSearchParams({ limit: String(logsState.limit) });
  if (logsState.level) q.set("level", logsState.level);

  let data;
  try {
    data = await api("/api/logs?" + q.toString());
  } catch (e) {
    $("#view-logs").innerHTML = `<div class="card"><div class="empty">Impossible de charger les logs : ${esc(e.message)}</div></div>`;
    return;
  }

  const entries = (data.logs || [])
    .map((l) => {
      const fields = l.fields ? Object.entries(l.fields).map(([k, v]) => `${k}=${typeof v === "object" ? JSON.stringify(v) : v}`).join("  ") : "";
      return `
        <div class="log-entry lvl-${esc(l.level)}">
          <span class="ts">${fmtTime(l.timestamp)}</span>
          <span class="lvl">${esc(l.level)}</span>
          <span>${esc(l.message)}</span>
          ${fields ? `<div class="fields">${esc(fields)}</div>` : ""}
        </div>`;
    })
    .join("");

  $("#view-logs").innerHTML = `
    <div class="card">
      <div class="toolbar">
        <select id="log-filter" class="field">
          ${["", "debug", "info", "warn", "error"].map((l) => `<option value="${l}" ${logsState.level === l ? "selected" : ""}>${l ? "≥ " + l : "Tous niveaux"}</option>`).join("")}
        </select>
        <span class="spacer"></span>
        <button class="btn btn-outline btn-small" id="log-refresh">Actualiser</button>
      </div>
      ${entries || `<div class="empty">Aucun log</div>`}
    </div>`;

  $("#log-filter").addEventListener("change", (e) => {
    logsState.level = e.target.value;
    loadLogs();
  });
  $("#log-refresh").addEventListener("click", loadLogs);
}

// ---------------------------------------------------------------------------
// Boot
// ---------------------------------------------------------------------------

switchTab("dashboard");
if (token) $("#btn-logout").classList.remove("hidden");
else requireLogin();
setInterval(() => {
  if ($("#view-dashboard").classList.contains("active") && token) loadDashboard();
}, 5000);