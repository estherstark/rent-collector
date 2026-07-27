package httpapi

import "github.com/gofiber/fiber/v2"

// registerPlayground serves a self-contained browser UI at GET /.
// It is a single HTML string with vanilla JS and zero external assets,
// talking to the API on the same origin (no CORS needed).
func (a *API) registerPlayground(app *fiber.App) {
	app.Get("/", func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return c.SendString(playgroundHTML)
	})
}

const playgroundHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Rent Collector — Playground</title>
<style>
  :root {
    --bg: #060e20;
    --surface: #101c33;
    --surface-2: #162542;
    --green: #00C16A;
    --red: #ff716c;
    --blue: #00D9FD;
    --text: #e2e8f0;
    --muted: #8fa3bf;
  }
  * { box-sizing: border-box; }
  body {
    margin: 0;
    background: var(--bg);
    color: var(--text);
    font-family: system-ui, -apple-system, "Segoe UI", sans-serif;
    line-height: 1.5;
  }
  .wrap { max-width: 900px; margin: 0 auto; padding: 24px 16px 64px; }
  h1 { font-size: 1.6rem; margin: 0 0 20px; }
  h2 { font-size: 1rem; margin: 0 0 12px; color: var(--text); }
  .grid { display: grid; grid-template-columns: 340px 1fr; gap: 16px; align-items: start; }
  @media (max-width: 760px) { .grid { grid-template-columns: 1fr; } }
  .col { display: flex; flex-direction: column; gap: 16px; }
  .card {
    background: var(--surface);
    border: 1px solid var(--surface-2);
    border-radius: 16px;
    padding: 18px;
  }
  label { display: block; font-size: .78rem; color: var(--muted); margin: 10px 0 4px; }
  input {
    width: 100%;
    background: var(--surface-2);
    border: 1px solid #223354;
    border-radius: 8px;
    color: var(--text);
    padding: 8px 10px;
    font: inherit;
    font-size: .9rem;
    color-scheme: dark;
  }
  input:focus { outline: 2px solid var(--green); outline-offset: -1px; border-color: transparent; }
  button {
    background: var(--green);
    color: #04140c;
    border: 0;
    border-radius: 8px;
    padding: 9px 14px;
    font: inherit;
    font-size: .9rem;
    font-weight: 600;
    cursor: pointer;
    margin-top: 12px;
  }
  button:hover { filter: brightness(1.1); }
  button.secondary { background: var(--surface-2); color: var(--text); border: 1px solid #2a3d63; }
  table { width: 100%; border-collapse: collapse; font-size: .85rem; }
  th { text-align: left; color: var(--muted); font-weight: 500; font-size: .75rem; padding: 6px 8px; border-bottom: 1px solid var(--surface-2); }
  td { padding: 7px 8px; border-bottom: 1px solid var(--surface-2); vertical-align: middle; }
  tr:last-child td { border-bottom: 0; }
  .num { text-align: right; font-variant-numeric: tabular-nums; }
  .pill { display: inline-block; padding: 2px 10px; border-radius: 999px; font-size: .72rem; font-weight: 600; }
  .pill.draft   { background: #2a3550; color: #aab6cc; }
  .pill.issued  { background: rgba(0,217,253,.15); color: var(--blue); }
  .pill.overdue { background: rgba(255,113,108,.15); color: var(--red); }
  .pill.paid    { background: rgba(0,193,106,.15); color: var(--green); }
  .pay-row { display: flex; gap: 6px; align-items: center; }
  .pay-row input { width: 90px; padding: 5px 8px; font-size: .8rem; }
  .pay-row button { margin: 0; padding: 5px 10px; font-size: .8rem; }
  .result {
    margin-top: 12px;
    padding: 10px 12px;
    border-radius: 8px;
    background: var(--surface-2);
    font-size: .9rem;
    display: none;
  }
  .result.show { display: block; }
  .result strong.created { color: var(--green); font-size: 1.05rem; }
  .result strong.skipped { color: var(--blue); font-size: 1.05rem; }
  .caption { margin-top: 8px; font-size: .75rem; color: var(--muted); }
  .muted { color: var(--muted); }
  .mono { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: .8rem; }
  .empty { color: var(--muted); font-size: .85rem; padding: 8px; }
  #toast {
    position: fixed;
    bottom: 20px;
    left: 50%;
    transform: translateX(-50%) translateY(80px);
    max-width: 90vw;
    background: var(--red);
    color: #2a0503;
    font-weight: 600;
    padding: 12px 18px;
    border-radius: 12px;
    box-shadow: 0 8px 30px rgba(0,0,0,.5);
    transition: transform .25s ease;
    z-index: 10;
  }
  #toast.ok { background: var(--green); color: #04140c; }
  #toast.show { transform: translateX(-50%) translateY(0); }
</style>
</head>
<body>
<div class="wrap">
  <h1>🏠 Rent Collector</h1>
  <div class="grid">
    <div class="col">
      <div class="card">
        <h2>New lease</h2>
        <form id="lease-form">
          <label for="lf-tenant">Tenant</label>
          <input id="lf-tenant" required placeholder="Somchai P.">
          <label for="lf-property">Property</label>
          <input id="lf-property" required placeholder="Sukhumvit 33 #804">
          <label for="lf-rent">Rent (THB / month)</label>
          <input id="lf-rent" type="number" min="1" step="0.01" required placeholder="18500">
          <label for="lf-due">Due day of month</label>
          <input id="lf-due" type="number" min="1" max="31" value="1" required>
          <button type="submit">Create lease</button>
        </form>
      </div>
      <div class="card">
        <h2>Billing actions</h2>
        <label for="bill-month">Billing month</label>
        <input id="bill-month" type="month" value="2026-08">
        <button id="btn-generate">Generate invoices</button>
        <div id="gen-result" class="result"></div>
        <div class="caption">กดซ้ำได้ — ระบบ idempotent จะขึ้น skipped แทน</div>
        <label for="asof-date" style="margin-top:16px">Late fees as of</label>
        <input id="asof-date" type="date" value="2026-08-15">
        <button id="btn-latefees" class="secondary">Apply late fees</button>
        <div id="fee-result" class="result"></div>
      </div>
    </div>
    <div class="col">
      <div class="card">
        <h2>Leases</h2>
        <div id="leases"></div>
      </div>
      <div class="card">
        <h2>Invoices <span id="inv-month-label" class="muted" style="font-weight:400;font-size:.8rem"></span></h2>
        <div id="invoices"></div>
      </div>
    </div>
  </div>
</div>
<div id="toast"></div>
<script>
"use strict";
const $ = (id) => document.getElementById(id);
let toastTimer;

function toast(msg, ok) {
  const el = $("toast");
  el.textContent = msg;
  el.className = ok ? "ok show" : "show";
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => el.classList.remove("show"), 4000);
}

async function api(path, opts) {
  const res = await fetch(path, opts);
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(res.status + ": " + (body.error || res.statusText));
  }
  return body;
}

function post(path, payload) {
  return api(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

const thb = (satang) =>
  (satang / 100).toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 });

function esc(s) {
  return String(s).replace(/[&<>"']/g, (ch) =>
    ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[ch]));
}

let leaseNames = {}; // lease id -> "tenant · property"

async function loadLeases() {
  const leases = await api("/leases");
  leaseNames = {};
  const host = $("leases");
  if (!leases.length) {
    host.innerHTML = '<div class="empty">No leases yet — create one on the left.</div>';
    return;
  }
  let rows = "";
  for (const l of leases) {
    leaseNames[l.id] = l.tenant + " · " + l.property;
    rows += "<tr><td>" + esc(l.tenant) + "</td><td>" + esc(l.property) +
      '</td><td class="num">' + thb(l.rent_satang) +
      '</td><td class="num">' + l.due_day + "</td></tr>";
  }
  host.innerHTML = '<table><thead><tr><th>Tenant</th><th>Property</th>' +
    '<th class="num">Rent (THB)</th><th class="num">Due day</th></tr></thead><tbody>' +
    rows + "</tbody></table>";
}

async function loadInvoices() {
  const month = $("bill-month").value;
  $("inv-month-label").textContent = month ? "— " + month : "";
  const invoices = await api("/invoices?month=" + encodeURIComponent(month));
  const host = $("invoices");
  if (!invoices.length) {
    host.innerHTML = '<div class="empty">No invoices for this month yet — generate them.</div>';
    return;
  }
  let rows = "";
  for (const inv of invoices) {
    const total = inv.amount_satang + inv.late_fee_satang;
    const remaining = Math.max(0, total - inv.paid_satang);
    const payable = inv.status === "issued" || inv.status === "overdue";
    const payCell = payable
      ? '<div class="pay-row"><input type="number" min="0.01" step="0.01" value="' +
        (remaining / 100).toFixed(2) + '" id="pay-' + esc(inv.id) + '">' +
        '<button onclick="pay(\'' + esc(inv.id) + '\')">Pay</button></div>'
      : '<span class="muted">—</span>';
    rows += "<tr>" +
      '<td class="mono" title="' + esc(inv.id) + '">' + esc(inv.id.slice(0, 8)) + "</td>" +
      "<td>" + esc(leaseNames[inv.lease_id] || inv.lease_id.slice(0, 8)) + "</td>" +
      '<td class="num">' + thb(total) + "</td>" +
      '<td class="num">' + thb(inv.paid_satang) + "</td>" +
      "<td>" + esc(inv.due_date.slice(0, 10)) + "</td>" +
      '<td><span class="pill ' + esc(inv.status) + '">' + esc(inv.status) + "</span></td>" +
      "<td>" + payCell + "</td></tr>";
  }
  host.innerHTML = '<table><thead><tr><th>ID</th><th>Lease</th>' +
    '<th class="num">Amount</th><th class="num">Paid</th><th>Due</th><th>Status</th><th>Pay (THB)</th>' +
    "</tr></thead><tbody>" + rows + "</tbody></table>";
}

const refresh = () => Promise.all([loadLeases().then(loadInvoices)]);

$("lease-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  try {
    await post("/leases", {
      tenant: $("lf-tenant").value.trim(),
      property: $("lf-property").value.trim(),
      rent_thb: parseFloat($("lf-rent").value),
      due_day: parseInt($("lf-due").value, 10),
    });
    e.target.reset();
    $("lf-due").value = "1";
    toast("Lease created", true);
    await refresh();
  } catch (err) { toast(err.message); }
});

$("btn-generate").addEventListener("click", async () => {
  try {
    const res = await post("/invoices/generate", { month: $("bill-month").value });
    const el = $("gen-result");
    el.innerHTML = '<strong class="created">' + res.created + " created</strong> · " +
      '<strong class="skipped">' + res.skipped + " skipped</strong>";
    el.classList.add("show");
    await refresh();
  } catch (err) { toast(err.message); }
});

$("btn-latefees").addEventListener("click", async () => {
  try {
    const res = await post("/invoices/late-fees", { as_of: $("asof-date").value });
    const el = $("fee-result");
    el.textContent = res.marked_overdue + " invoice(s) marked overdue";
    el.classList.add("show");
    await refresh();
  } catch (err) { toast(err.message); }
});

$("bill-month").addEventListener("change", () => loadInvoices().catch((e) => toast(e.message)));

window.pay = async (invoiceID) => {
  try {
    const amount = parseFloat($("pay-" + invoiceID).value);
    await post("/payments", {
      invoice_id: invoiceID,
      amount_thb: amount,
      idempotency_key: crypto.randomUUID(),
    });
    toast("Payment recorded", true);
    await refresh();
  } catch (err) { toast(err.message); }
};

refresh().catch((e) => toast(e.message));
</script>
</body>
</html>
`
