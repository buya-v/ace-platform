const express = require("express");
const cors = require("cors");
const crypto = require("node:crypto");
const fs = require("fs");
const path = require("path");

const PORT = process.env.PORT || 3010;

// DeepSeek is OpenAI-compatible. Keys may be comma-separated for failover.
const DEEPSEEK_API_KEYS = (process.env.DEEPSEEK_API_KEY || "").split(",").filter(Boolean);
const DEEPSEEK_BASE_URL = process.env.DEEPSEEK_BASE_URL || "https://api.deepseek.com";

// ─── Demo portal auth gate ─────────────────────────────────────────────────────
//
// A shared-password login gate with a HARD time cutoff, designed to sit behind an
// nginx `auth_request` against GET /api/_gate/check. nginx rewrites the public
// path /api/architect/ -> /api/, so the public-facing paths are:
//   - GET  /login                      (served directly, password page)
//   - POST /api/architect/_gate/login  -> Express route POST /api/_gate/login
//   - GET  /api/architect/_gate/check  -> Express route GET  /api/_gate/check
//
// The gate is additive: /api/health and /api/chat are NOT gated here (nginx gates
// /api/chat via auth_request). Fail-closed: if any of the three env vars below is
// unset/empty (or the cutoff is unparseable), /check denies (401) and /login 503s.

const COOKIE_NAME = "demo_auth";

const DEMO_PORTAL_PASSWORD = process.env.DEMO_PORTAL_PASSWORD || "";
const DEMO_AUTH_SECRET = process.env.DEMO_AUTH_SECRET || "";

// Parse the hard cutoff as a Unix epoch SECONDS integer. Reject anything that is
// not a positive integer (NaN, floats, negatives, garbage) -> gate stays unconfigured.
function parseEpochSeconds(raw) {
  if (raw === undefined || raw === null) return null;
  const s = String(raw).trim();
  if (!/^\d+$/.test(s)) return null;
  const n = Number(s);
  if (!Number.isInteger(n) || n <= 0) return null;
  return n;
}

const DEMO_AUTH_EXPIRES_AT = parseEpochSeconds(process.env.DEMO_AUTH_EXPIRES_AT);

// The gate is "configured" only when all three secrets are present and valid.
function gateConfigured() {
  return (
    DEMO_PORTAL_PASSWORD.length > 0 &&
    DEMO_AUTH_SECRET.length > 0 &&
    DEMO_AUTH_EXPIRES_AT !== null
  );
}

function nowSeconds() {
  return Math.floor(Date.now() / 1000);
}

// HMAC-SHA256(secret, String(exp)) as lowercase hex.
function signExp(exp) {
  return crypto
    .createHmac("sha256", DEMO_AUTH_SECRET)
    .update(String(exp))
    .digest("hex");
}

// Constant-time equality. Both inputs are SHA-256 hashed first so the comparison
// always runs over equal-length (32-byte) buffers — this avoids throwing on a
// length mismatch and does not leak the length of either operand.
function constantTimeEqual(a, b) {
  const ha = crypto.createHash("sha256").update(String(a)).digest();
  const hb = crypto.createHash("sha256").update(String(b)).digest();
  return crypto.timingSafeEqual(ha, hb);
}

// Minimal RFC-6265 Cookie header parser (no external dependency).
function parseCookies(header) {
  const out = {};
  if (!header) return out;
  for (const part of header.split(";")) {
    const idx = part.indexOf("=");
    if (idx < 0) continue;
    const name = part.slice(0, idx).trim();
    if (!name) continue;
    let value = part.slice(idx + 1).trim();
    if (value.startsWith('"') && value.endsWith('"')) {
      value = value.slice(1, -1);
    }
    out[name] = value;
  }
  return out;
}

// Validate the demo_auth cookie value. Valid IFF it is "<exp>.<hmac>", the hmac
// recomputes and matches via constant-time compare, exp equals the configured
// cutoff, and the cutoff has not passed. Side-effect-free and fast (auth_request
// calls this on every demo request).
function cookieValid(cookieValue) {
  if (!gateConfigured()) return false;
  if (typeof cookieValue !== "string" || cookieValue.length === 0) return false;

  const dot = cookieValue.indexOf(".");
  if (dot <= 0 || dot === cookieValue.length - 1) return false;

  const expPart = cookieValue.slice(0, dot);
  const hmacPart = cookieValue.slice(dot + 1);

  const exp = parseEpochSeconds(expPart);
  if (exp === null) return false;

  // Bind the cookie to the current configured cutoff so changing the cutoff
  // invalidates previously-issued cookies (enforces the HARD time window).
  if (exp !== DEMO_AUTH_EXPIRES_AT) return false;

  if (!constantTimeEqual(hmacPart, signExp(exp))) return false;

  // The hard cutoff: nobody is valid at or after the expiry instant.
  if (nowSeconds() >= exp) return false;

  return true;
}

// ─── Load knowledge base on startup ───────────────────────────────────────────

const knowledgeDir = path.join(__dirname, "knowledge");
const knowledgeFiles = fs
  .readdirSync(knowledgeDir)
  .filter((f) => f.endsWith(".md"))
  .sort();

let knowledgeContent = "";
for (const file of knowledgeFiles) {
  const filePath = path.join(knowledgeDir, file);
  const content = fs.readFileSync(filePath, "utf8");
  knowledgeContent += `\n\n---\n### ${file}\n\n${content}`;
}

console.log(
  `Loaded ${knowledgeFiles.length} knowledge files:`,
  knowledgeFiles.join(", ")
);
console.log(
  `DeepSeek backend: ${DEEPSEEK_API_KEYS.length} key(s) configured at ${DEEPSEEK_BASE_URL}`
);

if (gateConfigured()) {
  console.log(
    `Demo portal auth gate: ENABLED — access cutoff ${new Date(
      DEMO_AUTH_EXPIRES_AT * 1000
    ).toISOString()} (epoch ${DEMO_AUTH_EXPIRES_AT})`
  );
} else {
  console.log(
    "Demo portal auth gate: DISABLED (fail-closed) — set DEMO_PORTAL_PASSWORD, " +
      "DEMO_AUTH_SECRET and DEMO_AUTH_EXPIRES_AT (epoch seconds) to enable; " +
      "/api/_gate/check will deny and /api/_gate/login will 503 until then"
  );
}

// ─── System prompt ─────────────────────────────────────────────────────────────

const SYSTEM_PROMPT = `You are the Chief Architect of GarudaX, a multi-tenant AI-native securities exchange platform built to replace MillenniumIT at the Mongolian Stock Exchange (MSE).

You are presenting GarudaX to the MSE board, FRC regulators, and technical evaluation team.

Your persona:
- Confident and authoritative — you built this platform
- Technically deep — you know the codebase
- Strategically compelling — you can articulate why GarudaX is the right choice for MSE
- Honest about gaps — when asked about something GarudaX doesn't have, acknowledge it but pivot to strengths and roadmap
- Bilingual awareness — the audience may ask in Mongolian, respond in the language they use

When answering:
- Be concise but thorough (2-4 paragraphs max unless asked for detail)
- Use specific numbers and technical details from the knowledge base
- Compare to MillenniumIT when relevant
- Frame everything in terms of value to MSE
- If asked about pricing, say "GarudaX eliminates per-transaction licensing fees — MSE owns the platform"

KNOWLEDGE BASE:
${knowledgeContent}`;

// ─── Express app ───────────────────────────────────────────────────────────────

const app = express();

app.use(cors());
app.use(express.json());
app.use(express.urlencoded({ extended: false })); // for the login form POST

// ─── Demo portal auth gate routes ───────────────────────────────────────────────

// Minimal, self-contained, GarudaX-branded password page (inline CSS, no assets).
function renderLoginPage({ error = false, ended = false, unavailable = false } = {}) {
  let banner = "";
  if (unavailable) {
    banner = `<p class="msg">Demo access is not currently available.</p>`;
  } else if (ended) {
    banner = `<p class="msg">This demo access window has ended.</p>`;
  }

  const form =
    ended || unavailable
      ? ""
      : `${error ? `<div class="err">Incorrect password. Please try again.</div>` : ""}
      <form method="POST" action="/api/architect/_gate/login">
        <label for="password">Access password</label>
        <input id="password" name="password" type="password" autocomplete="current-password" autofocus required />
        <button type="submit">Enter demo</button>
      </form>`;

  return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>GarudaX Demo — Sign in</title>
<style>
  :root { color-scheme: dark; }
  * { box-sizing: border-box; }
  body {
    margin: 0; min-height: 100vh; display: flex; align-items: center; justify-content: center;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    background: radial-gradient(circle at 30% 20%, #16243a, #0a0f1a 70%); color: #e6edf6;
  }
  .card {
    width: 100%; max-width: 380px; margin: 24px; padding: 32px;
    background: #111927; border: 1px solid #233047; border-radius: 14px;
    box-shadow: 0 20px 50px rgba(0,0,0,0.45);
  }
  .brand { font-size: 22px; font-weight: 700; letter-spacing: 0.5px; }
  .brand span { color: #4f9dff; }
  .sub { margin: 6px 0 22px; font-size: 13px; color: #8c9bb3; }
  label { display: block; font-size: 13px; margin-bottom: 6px; color: #b6c2d6; }
  input {
    width: 100%; padding: 11px 12px; font-size: 15px; border-radius: 8px;
    border: 1px solid #2c3a52; background: #0c121d; color: #e6edf6; margin-bottom: 16px;
  }
  input:focus { outline: none; border-color: #4f9dff; }
  button {
    width: 100%; padding: 11px; font-size: 15px; font-weight: 600; cursor: pointer;
    border: none; border-radius: 8px; background: #2f6fed; color: #fff;
  }
  button:hover { background: #3b7bf5; }
  .err { background: #3a1620; border: 1px solid #6b2435; color: #ffb3c0;
    padding: 10px 12px; border-radius: 8px; font-size: 13px; margin-bottom: 16px; }
  .msg { background: #1b2738; border: 1px solid #2c3a52; color: #c6d2e6;
    padding: 14px; border-radius: 8px; font-size: 14px; }
  .foot { margin-top: 22px; font-size: 11px; color: #5f6e85; text-align: center; }
</style>
</head>
<body>
  <div class="card">
    <div class="brand">Garuda<span>X</span></div>
    <div class="sub">Demo portal — authorized access only</div>
    ${banner}
    ${form}
    <div class="foot">GarudaX — AI-native exchange platform</div>
  </div>
</body>
</html>`;
}

// GET /login — public password page.
app.get("/login", (req, res) => {
  res.set("Cache-Control", "no-store");
  res.type("html");
  if (!gateConfigured()) {
    return res.status(200).send(renderLoginPage({ unavailable: true }));
  }
  if (nowSeconds() >= DEMO_AUTH_EXPIRES_AT) {
    return res.status(200).send(renderLoginPage({ ended: true }));
  }
  res.status(200).send(renderLoginPage({ error: req.query.error === "1" }));
});

// POST /api/_gate/login — verify the shared password and issue the signed cookie.
// (Public path /api/architect/_gate/login after nginx rewrites /api/architect/ -> /api/.)
app.post("/api/_gate/login", (req, res) => {
  if (!gateConfigured()) {
    return res.status(503).json({ error: "demo gate not configured" });
  }
  if (nowSeconds() >= DEMO_AUTH_EXPIRES_AT) {
    return res.status(403).json({ error: "demo access window has ended" });
  }

  const password = (req.body && req.body.password) || "";
  if (!constantTimeEqual(password, DEMO_PORTAL_PASSWORD)) {
    return res.redirect(302, "/login?error=1");
  }

  const exp = DEMO_AUTH_EXPIRES_AT;
  const value = `${exp}.${signExp(exp)}`;
  const maxAge = Math.max(0, exp - nowSeconds());
  res.setHeader(
    "Set-Cookie",
    `${COOKIE_NAME}=${value}; Path=/; HttpOnly; Secure; SameSite=Lax; Max-Age=${maxAge}`
  );
  res.redirect(302, "/");
});

// GET /api/_gate/check — called by nginx auth_request on every demo request.
// 204 when the cookie is valid, 401 otherwise. Side-effect-free and fast.
app.get("/api/_gate/check", (req, res) => {
  res.set("Cache-Control", "no-store");
  const cookies = parseCookies(req.headers.cookie);
  if (cookieValid(cookies[COOKIE_NAME])) {
    return res.status(204).end();
  }
  res.status(401).end();
});

// GET /api/health
app.get("/api/health", (req, res) => {
  res.json({ status: "ok", service: "architect-bot" });
});

// POST /api/chat
app.post("/api/chat", async (req, res) => {
  const { message, history = [] } = req.body;

  if (!message) {
    return res.status(400).json({ error: "message is required" });
  }

  if (DEEPSEEK_API_KEYS.length === 0) {
    console.error("No DEEPSEEK_API_KEY configured");
    return res.json({
      response:
        "I apologize, my AI backend is not configured yet. Please try again shortly.",
      error: true,
    });
  }

  // Build OpenAI-style message list: system prompt + prior turns + new message.
  const messages = [
    { role: "system", content: SYSTEM_PROMPT },
    ...history.map((entry) => ({
      role: entry.role === "assistant" ? "assistant" : "user",
      content: entry.content,
    })),
    { role: "user", content: message },
  ];

  // Try keys and models in order of preference.
  const modelNames = ["deepseek-chat", "deepseek-reasoner"];
  let lastError = null;

  for (const apiKey of DEEPSEEK_API_KEYS) {
    for (const modelName of modelNames) {
      try {
        const resp = await fetch(`${DEEPSEEK_BASE_URL}/chat/completions`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${apiKey}`,
          },
          body: JSON.stringify({
            model: modelName,
            messages,
            temperature: 0.7,
            max_tokens: 4096,
            stream: false,
          }),
        });

        if (!resp.ok) {
          const body = await resp.text();
          throw new Error(`HTTP ${resp.status}: ${body.slice(0, 300)}`);
        }

        const data = await resp.json();
        const response = data?.choices?.[0]?.message?.content;
        if (!response) {
          throw new Error("empty completion from DeepSeek");
        }

        console.log(`Response from ${modelName} (${response.length} chars)`);
        res.json({ response });
        return;
      } catch (err) {
        console.error(
          `key=…${apiKey.slice(-4)} ${modelName} error:`,
          err?.message || err
        );
        lastError = err;
      }
    }
  }

  // All keys and models failed
  console.error("All DeepSeek models failed:", lastError?.message);
  res.json({
    response:
      "I apologize, I'm having trouble connecting to my AI backend. All models are currently unavailable. Please try again in a moment.",
    error: true,
  });
});

// ─── Start ─────────────────────────────────────────────────────────────────────

if (require.main === module) {
  app.listen(PORT, () => {
    console.log(`architect-bot listening on port ${PORT}`);
  });
}

// Exported for tests; the gate helpers capture env at module load, so tests that
// need a different config re-require this module with a cleared cache.
module.exports = {
  app,
  COOKIE_NAME,
  gateConfigured,
  parseEpochSeconds,
  parseCookies,
  constantTimeEqual,
  signExp,
  cookieValid,
  nowSeconds,
  DEMO_AUTH_EXPIRES_AT,
};
