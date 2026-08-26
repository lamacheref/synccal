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

// Confirmation modal
let confirmResolve = null;

function openConfirm(message, onConfirm) {
  $("#confirm-message").textContent = message;
  $("#confirm-modal").classList.add("show");
  return new Promise((resolve) => {
    confirmResolve = resolve;
  }).then((confirmed) => {
    if (confirmed && onConfirm) onConfirm();
  });
}

$("#confirm-ok").addEventListener("click", () => {
  $("#confirm-modal").classList.remove("show");
  if (confirmResolve) confirmResolve(true);
});

$("#confirm-cancel").addEventListener("click", () => {
  $("#confirm-modal").classList.remove("show");
  if (confirmResolve) confirmResolve(false);
});

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

const views = { dashboard: "#view-dashboard", config: "#view-config", events: "#view-events", logs: "#view-logs", plugins: "#view-plugins" };

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
  else if (name === "plugins") loadPlugins();
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

  let connRows = "";
  if (st.connections && st.connections.length) {
    connRows = st.connections
      .map(
        (c) => `
        <tr>
          <td class="mono src-url" title="${esc(c.source_url)}">${esc(c.source_url)}</td>
          <td>${esc(c.destination)}</td>
          <td>${c.last_error ? `<span class="badge err">${esc(c.last_error)}</span>` : `<span class="badge ok">OK</span>`}</td>
          <td>${c.events}</td>
          <td>${c.created}</td>
          <td>${c.updated}</td>
          <td>${c.deleted}</td>
          <td>${c.errors}</td>
          <td>${fmtTime(c.last_sync)}</td>
          <td>${c.last_duration_sec ? c.last_duration_sec.toFixed(1) + "s" : "—"}</td>
        </tr>`
      )
      .join("");
  } else {
    connRows = `<tr><td colspan="10" class="empty">Aucune connexion configurée</td></tr>`;
  }

  $("#view-dashboard").innerHTML = `
    <div class="section">
      <div class="section-header"><h2>État global</h2></div>
      <div class="section-body"><div class="grid cols-3">
      <div class="card stat"><div class="value ${state}">${stateLabel}</div><div class="label">État de la synchronisation</div></div>
      <div class="card stat"><div class="value">${esc(st.interval || "manuel")}</div><div class="label">Intervalle</div></div>
      <div class="card stat"><div class="value">${esc(lastSync)}</div><div class="label">Dernière synchronisation</div></div>
      <div class="card stat"><div class="value">${esc(dur)}</div><div class="label">Dernière durée</div></div>
      ${st.last_error ? `<div class="card stat"><div class="value err">${esc(st.last_error)}</div><div class="label">Dernière erreur</div></div>` : ""}
    </div></div>
    </div>
    <div class="section">
      <div class="section-header"><h2>Connexions</h2><span class="hint">Sources → Destinations</span></div>
      <div class="section-body"><div class="card" style="margin:0;box-shadow:none;">
      <div class="table-wrap">
        <table>
          <thead>
            <tr><th>Source</th><th>Destination</th><th>Statut</th><th>Événements</th><th>Créés</th><th>Mis à jour</th><th>Supprimés</th><th>Erreurs</th><th>Dernière sync</th><th>Durée</th></tr>
          </thead>
          <tbody>${connRows}</tbody>
        </table>
      </div></div>
    </div></div>
    </div>`;
}

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

let configCache = null;
let pluginCache = null;

async function fetchPlugins() {
  if (pluginCache) return pluginCache;
  try {
    const data = await api("/api/plugins");
    pluginCache = data.plugins || [];
    return pluginCache;
  } catch (e) {
    return [];
  }
}

function pluginOptions(kind, selected) {
  if (!pluginCache) return `<option value="${esc(selected)}" selected>${esc(selected)}</option>`;
  const list = pluginCache.filter((p) => p.kind === kind);
  if (!list.length) return `<option value="${esc(selected)}" selected>${esc(selected || "caldav")}</option>`;
  return list.map((p) => `<option value="${esc(p.type)}" ${p.type === selected ? "selected" : ""}>${esc(p.type)} — ${esc(p.name)}</option>`).join("");
}

function transformerTypeOptions(selected) {
  if (!pluginCache) return `<option value="${esc(selected)}" selected>${esc(selected)}</option>`;
  const list = pluginCache.filter((p) => p.kind === "transformer");
  if (!list.length) return `<option value="${esc(selected)}" selected>${esc(selected)}</option>`;
  return list.map((p) => `<option value="${esc(p.type)}" ${p.type === selected ? "selected" : ""}>${esc(p.type)} — ${esc(p.name)}</option>`).join("");
}

function renderTransformers(transformers, kind, idx) {
  if (!transformers || !transformers.length) return `<div class="empty">Aucun transformer — le pipeline par défaut (préfixe UID + filtre PRIVATE automatique) sera appliqué</div>`;
  return transformers.map((tr, ti) => `
    <div class="card" data-transformer-idx="${ti}" style="margin:8px 0;padding:12px;">
      <div class="card-header" style="margin-bottom:8px;">
        <span class="hint">Transformer ${ti+1}: ${esc(tr.type)}</span>
        <button type="button" class="btn btn-outline btn-small" data-action="delete-transformer" data-kind="${kind}" data-idx="${idx}" data-tidx="${ti}">🗑</button>
      </div>
      <div class="form-grid">
        <div class="field"><label>Type</label><select data-field="tr-type" data-kind="${kind}" data-idx="${idx}" data-tidx="${ti}">${transformerTypeOptions(tr.type)}</select></div>
        <div class="field"><label>Options (JSON)</label><input data-field="tr-options" data-kind="${kind}" data-idx="${idx}" data-tidx="${ti}" value="${esc(JSON.stringify(tr.options || {}))}" placeholder='{} ou {"prefix":"[Sync] "}'></div>
      </div>
      <div class="hint" style="margin-top:4px;font-size:12px;color:var(--text-secondary)">${esc((pluginCache||[]).find(p=>p.type===tr.type)?.description || "")}</div>
    </div>
  `).join("");
}


function sourceOptions(selected) {
  const srcs = configCache ? configCache.sources || [] : [];
  if (!srcs.length) return `<option value="" disabled selected>Aucune source — ajoutez d'abord une source</option>`;
  return srcs.map((s) => `<option value="${esc(s.name)}" ${s.name === selected ? "selected" : ""}>${esc(s.name)} (${esc(s.url)})</option>`).join("");
}

function refreshDestinationSourceOptions() {
  // Rebuild all destination source dropdowns when a source name changes
  $$("#destinations .card select[data-field='source']").forEach((sel) => {
    const cur = sel.value;
    // rebuild from current source names in DOM (not yet saved)
    const names = $$("#sources .card input[data-field='name']").map((i) => i.value.trim()).filter(Boolean);
    const urls = $$("#sources .card input[data-field='url']").map((i) => i.value.trim());
    sel.innerHTML = names.length ? names.map((n, idx) => `<option value="${esc(n)}" ${n === cur ? "selected" : ""}>${esc(n)} (${esc(urls[idx] || "")})</option>`).join("") : `<option value="" disabled selected>Aucune source</option>`;
    if (names.length && !names.includes(cur)) sel.value = names[0];
  });
}

async function loadConfigForm() {
  let cfg;
  try {
    cfg = await api("/api/config");
    configCache = cfg;
    await fetchPlugins();
  } catch (e) {
    $("#view-config").innerHTML = `<div class="card"><div class="empty">Impossible de charger la configuration : ${esc(e.message)}</div></div>`;
    return;
  }

  const srcFields = (cfg.sources || [])
    .map(
      (s, i) => `
      <div class="card" data-kind="source">
        <div class="card-header">
          <h2>Source ${i + 1}<span class="hint"> — ${esc(s.name || "")} (${esc(s.type || "caldav")})</span></h2>
          <button type="button" class="btn btn-outline btn-small btn-delete" data-action="delete-source" title="Supprimer cette source">🗑</button>
        </div>
        <div class="form-grid">
          <div class="field"><label>Nom <span class="hint">unique</span></label><input data-field="name" value="${esc(s.name)}" placeholder="ex: feries-fr"></div>
          <div class="field"><label>Type <span class="hint">plugin source</span></label><select data-field="type">${pluginOptions("source", s.type || "caldav")}</select></div>
          <div class="field"><label>URL source</label><input data-field="url" value="${esc(s.url)}" placeholder="https://example.com/calendar.ics"></div>
          <div class="field"><label>Utilisateur source <span class="hint">vide si public</span></label><input data-field="username" value="${esc(s.username)}" autocomplete="off"></div>
          <div class="field"><label>Mot de passe / token source <span class="hint">(laisser vide pour conserver)</span></label><input data-field="password" type="password" autocomplete="new-password"></div>
        </div>
        <div style="margin-top:12px;">
          <div class="card-header"><h3 style="font-size:14px;">Transformers source</h3><button type="button" class="btn btn-outline btn-small" data-action="add-transformer" data-kind="source" data-idx="${i}">+ Transformer</button></div>
          <div data-transformers="source" data-idx="${i}">${renderTransformers(s.transformers, "source", i)}</div>
        </div>
      </div>`
    )
    .join("");

  const destFields = (cfg.destinations || [])
    .map(
      (d, i) => `
      <div class="card" data-kind="destination">
        <div class="card-header">
          <h2>Destination ${i + 1}<span class="hint"> — ${esc(d.name || "")} (${esc(d.type || "caldav")})</span></h2>
          <button type="button" class="btn btn-outline btn-small btn-delete" data-action="delete-destination" title="Supprimer cette destination">🗑</button>
        </div>
        <div class="form-grid">
          <div class="field"><label>Nom <span class="hint">unique</span></label><input data-field="name" value="${esc(d.name)}" placeholder="ex: nextcloud-perso"></div>
          <div class="field"><label>Type <span class="hint">plugin destination</span></label><select data-field="type">${pluginOptions("destination", d.type || "caldav")}</select></div>
          <div class="field"><label>Source <span class="hint">liste déroulante des sources</span></label><select data-field="source">${sourceOptions(d.source)}</select></div>
          <div class="field"><label>URL destination</label><input data-field="url" value="${esc(d.url)}" placeholder="https://cloud.example.com/remote.php/dav/..."></div>
          <div class="field"><label>Utilisateur destination</label><input data-field="username" value="${esc(d.username)}" autocomplete="off"></div>
          <div class="field"><label>Mot de passe destination <span class="hint">(laisser vide pour conserver)</span></label><input data-field="password" type="password" autocomplete="new-password"></div>
        </div>
        <div style="margin-top:12px;">
          <div class="card-header"><h3 style="font-size:14px;">Transformers destination</h3><button type="button" class="btn btn-outline btn-small" data-action="add-transformer" data-kind="destination" data-idx="${i}">+ Transformer</button></div>
          <div data-transformers="destination" data-idx="${i}">${renderTransformers(d.transformers, "destination", i)}</div>
        </div>
      </div>`
    )
    .join("");

  $("#view-config").innerHTML = `
    <form id="config-form">
      <div class="section">
        <div class="section-header"><h2>Sources</h2><span class="hint">Calendriers sources — chaque source a un nom unique</span></div>
        <div class="section-body">
          <div id="sources">${srcFields || `<div class="card"><div class="empty">Aucune source — ajoutez-en une</div></div>`}</div>
          <div style="margin-top:12px; text-align:right;"><button type="button" class="btn btn-outline" id="btn-add-source">+ Ajouter une source</button></div>
        </div>
      </div>

      <div class="section">
        <div class="section-header"><h2>Destinations</h2><span class="hint">Chaque destination est jumelée à une source via la liste déroulante</span></div>
        <div class="section-body">
          <div id="destinations">${destFields || `<div class="card"><div class="empty">Aucune destination — ajoutez-en une</div></div>`}</div>
          <div style="margin-top:12px; text-align:right;"><button type="button" class="btn btn-outline" id="btn-add-destination">+ Ajouter une destination</button></div>
        </div>
      </div>

      <div class="section">
        <div class="section-header"><h2>Configuration</h2><span class="hint">Paramètres globaux — synchronisation, interface et journalisation</span></div>
        <div class="section-body">
          <div class="toolbar" style="justify-content:flex-end; margin-bottom:16px;">
            <button type="submit" class="btn btn-primary">Enregistrer la configuration</button>
          </div>

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

          <div class="toolbar" style="justify-content:flex-end; margin-top:16px;">
            <button type="submit" class="btn btn-primary">Enregistrer la configuration</button>
          </div>
        </div>
      </div>
    </form>`;

  $("#btn-add-source").addEventListener("click", () => addSourceCard());
  $("#btn-add-destination").addEventListener("click", () => addDestinationCard());
  $("#config-form").addEventListener("submit", saveConfig);

  // Refresh destination dropdowns when source names change
  $$("#sources input[data-field='name'], #sources input[data-field='url']").forEach((el) => {
    el.addEventListener("input", () => {
      // update cached names for dropdown preview
      const names = $$("#sources .card input[data-field='name']").map((i) => i.value.trim());
      const urls = $$("#sources .card input[data-field='url']").map((i) => i.value.trim());
      configCache.sources = names.map((n, idx) => ({ name: n, url: urls[idx] || "" }));
      refreshDestinationSourceOptions();
    });
  });

  // Delegated handlers for transformers and deletes
  document.addEventListener("click", (e) => {
    const btnAddTr = e.target.closest('[data-action="add-transformer"]');
    if (btnAddTr) {
      const kind = btnAddTr.dataset.kind;
      const idx = btnAddTr.dataset.idx;
      const wrap = document.querySelector(`[data-transformers="${kind}"][data-idx="${idx}"]`);
      if (wrap) {
        if (wrap.querySelector(".empty")) wrap.innerHTML = "";
        const trIdx = wrap.querySelectorAll("[data-field='tr-type']").length;
        const opts = transformerTypeOptions(pluginCache ? pluginCache.filter(p=>p.kind==="transformer")[0]?.type || "filter-private" : "filter-private");
        const div = document.createElement("div");
        div.className = "card";
        div.style.margin = "8px 0";
        div.style.padding = "12px";
        div.innerHTML = `
          <div class="card-header" style="margin-bottom:8px;">
            <span class="hint">Transformer ${trIdx+1}</span>
            <button type="button" class="btn btn-outline btn-small" data-action="delete-transformer" data-kind="${kind}" data-idx="${idx}" data-tidx="${trIdx}">🗑</button>
          </div>
          <div class="form-grid">
            <div class="field"><label>Type</label><select data-field="tr-type" data-kind="${kind}" data-idx="${idx}" data-tidx="${trIdx}">${opts}</select></div>
            <div class="field"><label>Options (JSON)</label><input data-field="tr-options" data-kind="${kind}" data-idx="${idx}" data-tidx="${trIdx}" placeholder='{}'></div>
          </div>`;
        wrap.appendChild(div);
      }
      return;
    }
    const btnDelTr = e.target.closest('[data-action="delete-transformer"]');
    if (btnDelTr) {
      const card = btnDelTr.closest(".card");
      if (card) card.remove();
      return;
    }
    const btnSrc = e.target.closest('[data-action="delete-source"]');
    if (btnSrc) {
      const card = btnSrc.closest(".card");
      if (card) openConfirm("Supprimer cette source ? Les destinations liées deviendront invalides.", () => { card.remove(); refreshDestinationSourceOptions(); });
      return;
    }
    const btnDst = e.target.closest('[data-action="delete-destination"]');
    if (btnDst) {
      const card = btnDst.closest(".card");
      if (card) openConfirm("Supprimer cette destination ?", () => card.remove());
    }
  });
}

function addSourceCard() {
  const wrap = $("#sources");
  const empty = wrap.querySelector(".empty");
  if (empty) wrap.innerHTML = "";
  const card = document.createElement("div");
  card.className = "card";
  card.dataset.kind = "source";
  const typeOpts = pluginCache ? pluginCache.filter(p=>p.kind==="source").map(p=>`<option value="${esc(p.type)}">${esc(p.type)} — ${esc(p.name)}</option>`).join("") : `<option value="caldav" selected>caldav</option>`;
  card.innerHTML = `
    <div class="card-header">
      <h2>Nouvelle source</h2>
      <button type="button" class="btn btn-outline btn-small btn-delete" data-action="delete-source" title="Supprimer cette source">🗑</button>
    </div>
    <div class="form-grid">
      <div class="field"><label>Nom</label><input data-field="name" value="" placeholder="ex: feries-fr"></div>
      <div class="field"><label>Type</label><select data-field="type">${typeOpts}</select></div>
      <div class="field"><label>URL source</label><input data-field="url" value="" placeholder="https://example.com/calendar.ics"></div>
      <div class="field"><label>Utilisateur source</label><input data-field="username" value="" autocomplete="off"></div>
      <div class="field"><label>Mot de passe / token source</label><input data-field="password" type="password" autocomplete="new-password"></div>
    </div>
    <div style="margin-top:12px;">
      <div class="card-header"><h3 style="font-size:14px;">Transformers source</h3><button type="button" class="btn btn-outline btn-small" data-action="add-transformer" data-kind="source" data-idx="${wrap.querySelectorAll(".card").length}">+ Transformer</button></div>
      <div data-transformers="source" data-idx="${wrap.querySelectorAll(".card").length}"><div class="empty">Aucun transformer</div></div>
    </div>`;
  wrap.appendChild(card);
  // wire name change to refresh dropdowns
  card.querySelectorAll("input[data-field='name'], input[data-field='url']").forEach((el) => {
    el.addEventListener("input", () => {
      const names = $$("#sources .card input[data-field='name']").map((i) => i.value.trim());
      const urls = $$("#sources .card input[data-field='url']").map((i) => i.value.trim());
      configCache.sources = names.map((n, idx) => ({ name: n, url: urls[idx] || "" }));
      refreshDestinationSourceOptions();
    });
  });
  refreshDestinationSourceOptions();
}

function addDestinationCard() {
  const wrap = $("#destinations");
  const empty = wrap.querySelector(".empty");
  if (empty) wrap.innerHTML = "";
  const card = document.createElement("div");
  card.className = "card";
  card.dataset.kind = "destination";
  const opts = sourceOptions("");
  const typeOpts = pluginCache ? pluginCache.filter(p=>p.kind==="destination").map(p=>`<option value="${esc(p.type)}">${esc(p.type)} — ${esc(p.name)}</option>`).join("") : `<option value="caldav" selected>caldav</option>`;
  card.innerHTML = `
    <div class="card-header">
      <h2>Nouvelle destination</h2>
      <button type="button" class="btn btn-outline btn-small btn-delete" data-action="delete-destination" title="Supprimer cette destination">🗑</button>
    </div>
    <div class="form-grid">
      <div class="field"><label>Nom</label><input data-field="name" value="" placeholder="ex: nextcloud-perso"></div>
      <div class="field"><label>Type</label><select data-field="type">${typeOpts}</select></div>
      <div class="field"><label>Source</label><select data-field="source">${opts}</select></div>
      <div class="field"><label>URL destination</label><input data-field="url" value="" placeholder="https://cloud.example.com/remote.php/dav/..."></div>
      <div class="field"><label>Utilisateur destination</label><input data-field="username" value="" autocomplete="off"></div>
      <div class="field"><label>Mot de passe destination</label><input data-field="password" type="password" autocomplete="new-password"></div>
    </div>
    <div style="margin-top:12px;">
      <div class="card-header"><h3 style="font-size:14px;">Transformers destination</h3><button type="button" class="btn btn-outline btn-small" data-action="add-transformer" data-kind="destination" data-idx="${wrap.querySelectorAll(".card").length}">+ Transformer</button></div>
      <div data-transformers="destination" data-idx="${wrap.querySelectorAll(".card").length}"><div class="empty">Aucun transformer</div></div>
    </div>`;
  wrap.appendChild(card);
}

function collectConfig() {
  const cfg = JSON.parse(JSON.stringify(configCache || {}));
  cfg.sources = $$("#sources .card[data-kind='source']").map((card) => {
    const s = { name: "", type: "caldav", url: "", username: "" };
    card.querySelectorAll("[data-field]").forEach((input) => {
      const key = input.dataset.field;
      if (key === "password") {
        if (input.value) s.password = input.value;
      } else if (key.startsWith("tr-")) {
        // skip transformer fields, handled separately
      } else {
        s[key] = input.value.trim();
      }
    });
    // collect transformers for this source
    const trWrap = card.querySelector('[data-transformers="source"]');
    if (trWrap) {
      s.transformers = Array.from(trWrap.querySelectorAll("[data-field='tr-type']")).map((sel, idx) => {
        const type = sel.value;
        const optsInput = trWrap.querySelector(`[data-field='tr-options'][data-tidx="${idx}"]`);
        let opts = {};
        if (optsInput && optsInput.value.trim()) {
          try { opts = JSON.parse(optsInput.value); } catch(e) {
            // try key=value parsing
            optsInput.value.split(",").forEach(pair => {
              const [k,v] = pair.split("=").map(s=>s.trim());
              if (k) opts[k]=v||"";
            });
          }
        }
        return { type, options: opts };
      }).filter(tr => tr.type);
    }
    return s;
  }).filter((s) => s.name || s.url);

  cfg.destinations = $$("#destinations .card[data-kind='destination']").map((card) => {
    const d = { name: "", type: "caldav", url: "", username: "", source: "" };
    card.querySelectorAll("[data-field]").forEach((input) => {
      const key = input.dataset.field;
      if (key === "password") {
        if (input.value) d.password = input.value;
      } else if (key.startsWith("tr-")) {
        // skip
      } else {
        d[key] = input.value.trim();
      }
    });
    const trWrap = card.querySelector('[data-transformers="destination"]');
    if (trWrap) {
      d.transformers = Array.from(trWrap.querySelectorAll("[data-field='tr-type']")).map((sel, idx) => {
        const type = sel.value;
        const optsInput = trWrap.querySelector(`[data-field='tr-options'][data-tidx="${idx}"]`);
        let opts = {};
        if (optsInput && optsInput.value.trim()) {
          try { opts = JSON.parse(optsInput.value); } catch(e) {
            optsInput.value.split(",").forEach(pair => {
              const [k,v] = pair.split("=").map(s=>s.trim());
              if (k) opts[k]=v||"";
            });
          }
        }
        return { type, options: opts };
      }).filter(tr => tr.type);
    }
    return d;
  }).filter((d) => d.name || d.url);

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

  const destOptions = (cfg ? cfg.destinations || [] : [])
    .map((d) => {
      return d.name ? `<option value="${esc(d.name)}" ${eventsState.dest === d.name ? "selected" : ""}>${esc(d.name)}</option>` : "";
    })
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
    <div class="section">
      <div class="section-header"><h2>Événements synchronisés</h2><span class="hint">Filtre par destination</span></div>
      <div class="section-body"><div class="card" style="margin:0;box-shadow:none;">
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
      </div></div>
    </div></div>
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
    <div class="section">
      <div class="section-header"><h2>Logs</h2><span class="hint">Flux structuré — filtre par niveau</span></div>
      <div class="section-body"><div class="card" style="margin:0;box-shadow:none;">
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
// Plugins
// ---------------------------------------------------------------------------

async function loadPlugins() {
  let plugins;
  try {
    const data = await api("/api/plugins");
    plugins = data.plugins || [];
    pluginCache = plugins;
  } catch (e) {
    $("#view-plugins").innerHTML = `<div class="card"><div class="empty">Impossible de charger les plugins : ${esc(e.message)}</div></div>`;
    return;
  }
  const byKind = { source: [], destination: [], transformer: [] };
  plugins.forEach(p => { if (byKind[p.kind]) byKind[p.kind].push(p); });
  // Fetch installed archives
  let installed = [];
  try {
    const inst = await api("/api/plugins/installed");
    installed = inst.installed || [];
  } catch(e) {}

  $("#view-plugins").innerHTML = `
    <div class="section">
      <div class="section-header"><h2>Installer un plugin</h2><span class="hint">Upload d'une archive (.zip, .tar.gz) contenant le plugin</span></div>
      <div class="section-body">
        <div class="upload-area" id="plugin-drop">
          <p style="margin-bottom:8px;">Glissez-déposez une archive ici ou</p>
          <input type="file" id="plugin-file" accept=".zip,.tar,.tar.gz,.tgz,.gz" style="display:none">
          <button class="btn btn-outline" id="btn-pick-plugin">Choisir un fichier</button>
          <button class="btn btn-primary" id="btn-upload-plugin" style="margin-left:8px;">Uploader</button>
          <div id="upload-status" class="hint" style="margin-top:8px;"></div>
        </div>
        <div class="card"><h3>Archives installées</h3>
          ${installed.length ? `<table><thead><tr><th>Fichier</th><th>Taille</th><th>Date</th></tr></thead><tbody>${installed.map(f=>`<tr><td class="mono">${esc(f.name)}</td><td>${f.size} o</td><td>${esc(f.mod)}</td></tr>`).join("")}</tbody></table>` : `<div class="empty">Aucune archive installée — les plugins intégrés sont ci-dessous</div>`}
        </div>
      </div>
    </div>

    <div class="section">
      <div class="section-header"><h2>Connecteurs Source</h2><span class="hint">SourceConnector — type à sélectionner dans chaque source</span></div>
      <div class="section-body">
        ${byKind.source.map(p=>`<div class="card"><h3>${esc(p.type)} — ${esc(p.name)}</h3><p style="color:var(--text-secondary)">${esc(p.description)}</p><span class="badge ok">${esc(p.kind)}</span></div>`).join("") || `<div class="card"><div class="empty">Aucun connecteur source</div></div>`}
      </div>
    </div>

    <div class="section">
      <div class="section-header"><h2>Connecteurs Destination</h2><span class="hint">DestinationConnector</span></div>
      <div class="section-body">
        ${byKind.destination.map(p=>`<div class="card"><h3>${esc(p.type)} — ${esc(p.name)}</h3><p style="color:var(--text-secondary)">${esc(p.description)}</p><span class="badge ok">${esc(p.kind)}</span></div>`).join("") || `<div class="card"><div class="empty">Aucun connecteur destination</div></div>`}
      </div>
    </div>

    <div class="section">
      <div class="section-header"><h2>Transformers</h2><span class="hint">Pipeline — configuré par destination/source</span></div>
      <div class="section-body">
        ${byKind.transformer.map(p=>`<div class="card"><h3>${esc(p.type)} — ${esc(p.name)}</h3><p style="color:var(--text-secondary)">${esc(p.description)}</p><span class="badge ok">${esc(p.kind)}</span></div>`).join("") || `<div class="card"><div class="empty">Aucun transformer</div></div>`}
      </div>
    </div>
  `;
  // Wire upload
  const pickBtn = $("#btn-pick-plugin");
  const fileInput = $("#plugin-file");
  const uploadBtn = $("#btn-upload-plugin");
  const statusEl = $("#upload-status");
  const dropArea = $("#plugin-drop");
  if (pickBtn && fileInput) {
    pickBtn.addEventListener("click", () => fileInput.click());
    fileInput.addEventListener("change", () => {
      if (fileInput.files.length) statusEl.textContent = "Fichier sélectionné: " + fileInput.files[0].name;
    });
    if (dropArea) {
      dropArea.addEventListener("dragover", (e) => { e.preventDefault(); dropArea.classList.add("dragover"); });
      dropArea.addEventListener("dragleave", () => dropArea.classList.remove("dragover"));
      dropArea.addEventListener("drop", (e) => {
        e.preventDefault(); dropArea.classList.remove("dragover");
        if (e.dataTransfer.files.length) {
          fileInput.files = e.dataTransfer.files;
          statusEl.textContent = "Fichier sélectionné: " + fileInput.files[0].name;
        }
      });
    }
  }
  if (uploadBtn && fileInput) {
    uploadBtn.addEventListener("click", async () => {
      if (!fileInput.files.length) { snack("Sélectionnez d'abord une archive", true); return; }
      const fd = new FormData();
      fd.append("archive", fileInput.files[0]);
      uploadBtn.disabled = true;
      statusEl.textContent = "Upload en cours...";
      try {
        const headers = {};
        if (token) headers["Authorization"] = "Bearer " + token;
        const res = await fetch("/api/plugins/upload", { method: "POST", headers, body: fd });
        const data = await res.json().catch(()=>({}));
        if (!res.ok) throw new Error(data.error || res.statusText);
        snack("Plugin installé: " + data.filename);
        statusEl.textContent = "Installé: " + data.filename;
        setTimeout(loadPlugins, 800);
      } catch(e) {
        snack("Échec upload: " + e.message, true);
        statusEl.textContent = "Erreur: " + e.message;
      } finally {
        uploadBtn.disabled = false;
      }
    });
  }

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