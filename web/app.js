const $ = (id) => document.getElementById(id);
let selected = null;

function authHeaders(extra) {
  const headers = Object.assign({}, extra || {});
  const token = sessionStorage.getItem("platformToken");
  if (token) headers.Authorization = "Bearer " + token;
  return headers;
}

async function api(path, options) {
  const opts = Object.assign({}, options || {});
  opts.headers = authHeaders(opts.headers);
  const res = await fetch(path, opts);
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || res.statusText);
  }
  const type = res.headers.get("content-type") || "";
  if (type.includes("json")) return res.json();
  return res.text();
}

function badge(text) {
  const safe = String(text || "unknown");
  return `<span class="badge ${safe}">${safe}</span>`;
}

function bindToken() {
  const input = $("token");
  if (!input) return;
  input.value = sessionStorage.getItem("platformToken") || "";
  input.addEventListener("change", () => {
    const value = input.value.trim();
    if (value) sessionStorage.setItem("platformToken", value);
    else sessionStorage.removeItem("platformToken");
    refresh().catch((err) => console.error(err));
  });
}

async function loadMeta() {
  const meta = await api("/api/v1/meta");
  if (meta.grafana_url) $("grafana").href = meta.grafana_url;
  if (meta.jaeger_url) $("jaeger").href = meta.jaeger_url;
}

async function loadSLOs() {
  const data = await api("/api/v1/slos");
  const rows = (data.results || []).map((r) => `<tr>
    <td>${r.service}</td><td>${r.slo}</td><td>${r.window}</td>
    <td>${Number(r.current).toFixed(3)}%</td><td>${r.target}%</td>
    <td>${Number(r.burn_rate).toFixed(2)}</td>
    <td>${Number(r.error_budget_remaining).toFixed(1)}%</td>
    <td>${badge(r.status)}</td></tr>`).join("");
  $("slos").innerHTML = `<table><thead><tr><th>Service</th><th>SLO</th><th>Window</th><th>SLI</th><th>Target</th><th>Burn</th><th>Budget left</th><th>Status</th></tr></thead><tbody>${rows || "<tr><td colspan='8'>No evaluations yet. Traffic and a tick are required.</td></tr>"}</tbody></table>`;
}

async function loadIncidents() {
  const data = await api("/api/v1/incidents");
  const items = data.incidents || [];
  $("incidents").innerHTML = items.map((inc) => `<button class="incident" data-id="${inc.incident_id}">
    <strong>${inc.incident_id}</strong> ${badge(inc.severity)} ${badge(inc.status)}<br/>
    <span class="muted">${inc.service} — ${inc.title}</span>
  </button>`).join("") || "<p class='muted'>No incidents.</p>";
  for (const button of $("incidents").querySelectorAll("button")) {
    button.onclick = () => show(button.dataset.id);
  }
}

async function loadFaults() {
  const data = await api("/api/v1/demo/faults");
  const enabled = new Set((data.faults || []).filter((f) => f.enabled).map((f) => f.name));
  $("faults").innerHTML = (data.catalog || []).map((f) => `<button data-fault="${f.name}" data-on="${enabled.has(f.name)}">${enabled.has(f.name) ? "Disable" : "Enable"} ${f.name}</button>`).join("");
  for (const button of $("faults").querySelectorAll("button")) {
    button.onclick = async () => {
      const name = button.dataset.fault;
      if (button.dataset.on === "true") {
        await api(`/api/v1/demo/faults/${name}`, { method: "DELETE" });
      } else {
        await api("/api/v1/demo/faults", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ name }) });
      }
      await refresh();
    };
  }
}

async function show(id) {
  selected = id;
  const inc = await api(`/api/v1/incidents/${id}`);
  $("detail").hidden = false;
  $("detail-title").textContent = `${inc.incident_id} · ${inc.service}`;
  const label = inc.analysis ? inc.analysis.confidence_label : "insufficient_evidence";
  const ai = inc.analysis ? inc.analysis.ai_status : "not_requested";
  $("grades").innerHTML = `${badge(inc.severity)} ${badge(inc.status)} ${badge(label)} <span class="muted">AI ${ai}</span>`;
  $("summary").textContent = inc.summary || "No summary recorded.";
  $("timeline").innerHTML = (inc.events || []).map((e) => `<li><code>${e.at}</code> ${e.kind} — ${e.message}</li>`).join("");
  $("hypotheses").innerHTML = ((inc.analysis && inc.analysis.hypotheses) || []).map((h) => `<p>${badge(h.grade)} ${h.ai_generated ? badge("ai_generated") : ""} ${h.statement}</p>`).join("") || "<p class='muted'>No hypotheses.</p>";
  $("evidence").innerHTML = ((inc.analysis && inc.analysis.evidence) || []).map((e) => `<li>${badge(e.grade)} <strong>${e.kind}</strong> ${e.summary}</li>`).join("") || "<li>No evidence.</li>";
  $("postmortem-body").textContent = "";
}

$("analyze").onclick = async () => {
  if (!selected) return;
  await api(`/api/v1/incidents/${selected}/analyze`, { method: "POST" });
  await show(selected);
};
$("postmortem").onclick = async () => {
  if (!selected) return;
  $("postmortem-body").textContent = await api(`/api/v1/incidents/${selected}/postmortem`);
};

async function refresh() {
  await Promise.all([loadSLOs(), loadIncidents(), loadFaults()]);
  if (selected) await show(selected);
}

bindToken();
loadMeta().catch(() => {});
refresh().catch((err) => {
  $("slos").textContent = err.message;
});
setInterval(() => refresh().catch(() => {}), 10000);
