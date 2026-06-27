// Tests for the demo portal auth gate (R031).
// Uses Node 22 built-in test runner (`node --test`) — zero external dependencies.
//
//   cd src/architect-bot && node --test
//
// The gate helpers in server.js capture env vars at module load, so to exercise
// different configurations (enabled / disabled / expired) we re-require the module
// with a cleared require cache after mutating process.env.

const { test } = require("node:test");
const assert = require("node:assert/strict");
const path = require("node:path");

const SERVER_PATH = path.join(__dirname, "server.js");

const FUTURE = Math.floor(Date.now() / 1000) + 7 * 24 * 3600; // +7 days
const PAST = Math.floor(Date.now() / 1000) - 60; // 1 min ago
const PASSWORD = "open-sesame-1234";
const SECRET = "test-signing-secret-deadbeef";

// Load a fresh instance of server.js with the given env overrides applied.
function loadServer(env = {}) {
  const saved = {};
  const keys = [
    "DEMO_PORTAL_PASSWORD",
    "DEMO_AUTH_SECRET",
    "DEMO_AUTH_EXPIRES_AT",
  ];
  for (const k of keys) saved[k] = process.env[k];
  for (const k of keys) delete process.env[k];
  for (const [k, v] of Object.entries(env)) process.env[k] = String(v);

  delete require.cache[require.resolve(SERVER_PATH)];
  const mod = require(SERVER_PATH);

  // restore env so we don't leak between loads
  for (const k of keys) {
    if (saved[k] === undefined) delete process.env[k];
    else process.env[k] = saved[k];
  }
  return mod;
}

// Start the exported app on an ephemeral port; returns {base, close}.
function startServer(mod) {
  return new Promise((resolve) => {
    const server = mod.app.listen(0, "127.0.0.1", () => {
      const { port } = server.address();
      resolve({
        base: `http://127.0.0.1:${port}`,
        close: () => new Promise((r) => server.close(r)),
      });
    });
  });
}

const enabledEnv = {
  DEMO_PORTAL_PASSWORD: PASSWORD,
  DEMO_AUTH_SECRET: SECRET,
  DEMO_AUTH_EXPIRES_AT: FUTURE,
};

// ─── Pure helpers ────────────────────────────────────────────────────────────

test("parseEpochSeconds accepts positive integers, rejects garbage", () => {
  const { parseEpochSeconds } = loadServer(enabledEnv);
  assert.equal(parseEpochSeconds("1700000000"), 1700000000);
  assert.equal(parseEpochSeconds(" 42 "), 42);
  assert.equal(parseEpochSeconds("0"), null);
  assert.equal(parseEpochSeconds("-5"), null);
  assert.equal(parseEpochSeconds("3.14"), null);
  assert.equal(parseEpochSeconds("abc"), null);
  assert.equal(parseEpochSeconds(""), null);
  assert.equal(parseEpochSeconds(undefined), null);
  assert.equal(parseEpochSeconds(null), null);
});

test("parseCookies handles missing, multiple and quoted values", () => {
  const { parseCookies } = loadServer(enabledEnv);
  assert.deepEqual(parseCookies(undefined), {});
  assert.deepEqual(parseCookies(""), {});
  assert.deepEqual(parseCookies("a=1; b=2"), { a: "1", b: "2" });
  assert.deepEqual(parseCookies('demo_auth="x.y"'), { demo_auth: "x.y" });
  assert.equal(parseCookies("demo_auth=123.abc; other=z").demo_auth, "123.abc");
});

test("constantTimeEqual matches equal strings, rejects different/empty", () => {
  const { constantTimeEqual } = loadServer(enabledEnv);
  assert.equal(constantTimeEqual("hunter2", "hunter2"), true);
  assert.equal(constantTimeEqual("hunter2", "hunter3"), false);
  // length mismatch must not throw
  assert.equal(constantTimeEqual("short", "a-much-longer-value"), false);
  assert.equal(constantTimeEqual("", ""), true);
  assert.equal(constantTimeEqual("x", ""), false);
});

test("signExp is deterministic and depends on the secret", () => {
  const a = loadServer(enabledEnv);
  const sig1 = a.signExp(FUTURE);
  const sig2 = a.signExp(FUTURE);
  assert.equal(sig1, sig2);
  assert.match(sig1, /^[0-9a-f]{64}$/);
  const b = loadServer({ ...enabledEnv, DEMO_AUTH_SECRET: "different-secret" });
  assert.notEqual(b.signExp(FUTURE), sig1);
});

// ─── cookieValid ─────────────────────────────────────────────────────────────

test("cookieValid accepts a correctly signed, unexpired cookie", () => {
  const m = loadServer(enabledEnv);
  const value = `${FUTURE}.${m.signExp(FUTURE)}`;
  assert.equal(m.cookieValid(value), true);
});

test("cookieValid rejects tampered hmac, wrong exp, and malformed values", () => {
  const m = loadServer(enabledEnv);
  assert.equal(m.cookieValid(`${FUTURE}.${m.signExp(FUTURE)}x`), false);
  assert.equal(m.cookieValid(`${FUTURE}.deadbeef`), false);
  // exp not equal to configured cutoff (even if self-consistently signed)
  const otherExp = FUTURE + 999;
  assert.equal(m.cookieValid(`${otherExp}.${m.signExp(otherExp)}`), false);
  assert.equal(m.cookieValid("nodot"), false);
  assert.equal(m.cookieValid(`.${m.signExp(FUTURE)}`), false);
  assert.equal(m.cookieValid(`${FUTURE}.`), false);
  assert.equal(m.cookieValid(""), false);
  assert.equal(m.cookieValid(undefined), false);
});

test("cookieValid rejects everything once the cutoff has passed", () => {
  const m = loadServer({ ...enabledEnv, DEMO_AUTH_EXPIRES_AT: PAST });
  const value = `${PAST}.${m.signExp(PAST)}`;
  assert.equal(m.cookieValid(value), false);
});

test("cookieValid fails closed when the gate is unconfigured", () => {
  const m = loadServer({
    DEMO_AUTH_SECRET: SECRET,
    DEMO_AUTH_EXPIRES_AT: FUTURE,
  }); // no password
  assert.equal(m.gateConfigured(), false);
  assert.equal(m.cookieValid("anything"), false);
});

// ─── HTTP endpoints ───────────────────────────────────────────────────────────

test("GET /api/_gate/check returns 204 for a valid cookie, 401 otherwise", async () => {
  const m = loadServer(enabledEnv);
  const { base, close } = await startServer(m);
  try {
    const good = await fetch(`${base}/api/_gate/check`, {
      headers: { cookie: `${m.COOKIE_NAME}=${FUTURE}.${m.signExp(FUTURE)}` },
    });
    assert.equal(good.status, 204);

    const none = await fetch(`${base}/api/_gate/check`);
    assert.equal(none.status, 401);

    const bad = await fetch(`${base}/api/_gate/check`, {
      headers: { cookie: `${m.COOKIE_NAME}=${FUTURE}.bogus` },
    });
    assert.equal(bad.status, 401);
  } finally {
    await close();
  }
});

test("POST /api/_gate/login sets a signed cookie and redirects on correct password", async () => {
  const m = loadServer(enabledEnv);
  const { base, close } = await startServer(m);
  try {
    const resp = await fetch(`${base}/api/_gate/login`, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({ password: PASSWORD }).toString(),
      redirect: "manual",
    });
    assert.equal(resp.status, 302);
    assert.equal(resp.headers.get("location"), "/");
    const setCookie = resp.headers.get("set-cookie");
    assert.match(setCookie, new RegExp(`^${m.COOKIE_NAME}=${FUTURE}\\.[0-9a-f]{64}`));
    assert.match(setCookie, /HttpOnly/);
    assert.match(setCookie, /Secure/);
    assert.match(setCookie, /SameSite=Lax/);
    assert.match(setCookie, /Path=\//);
    assert.match(setCookie, /Max-Age=\d+/);
  } finally {
    await close();
  }
});

test("POST /api/_gate/login redirects to /login?error=1 on wrong password", async () => {
  const m = loadServer(enabledEnv);
  const { base, close } = await startServer(m);
  try {
    const resp = await fetch(`${base}/api/_gate/login`, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({ password: "wrong" }).toString(),
      redirect: "manual",
    });
    assert.equal(resp.status, 302);
    assert.equal(resp.headers.get("location"), "/login?error=1");
    assert.equal(resp.headers.get("set-cookie"), null);
  } finally {
    await close();
  }
});

test("POST /api/_gate/login returns 403 after the cutoff", async () => {
  const m = loadServer({ ...enabledEnv, DEMO_AUTH_EXPIRES_AT: PAST });
  const { base, close } = await startServer(m);
  try {
    const resp = await fetch(`${base}/api/_gate/login`, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({ password: PASSWORD }).toString(),
      redirect: "manual",
    });
    assert.equal(resp.status, 403);
  } finally {
    await close();
  }
});

test("login 503s and check 401s when the gate is unconfigured", async () => {
  const m = loadServer({}); // nothing set
  const { base, close } = await startServer(m);
  try {
    const login = await fetch(`${base}/api/_gate/login`, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({ password: "x" }).toString(),
      redirect: "manual",
    });
    assert.equal(login.status, 503);

    const check = await fetch(`${base}/api/_gate/check`);
    assert.equal(check.status, 401);
  } finally {
    await close();
  }
});

test("GET /login serves the form when enabled, ended message after cutoff", async () => {
  const enabled = loadServer(enabledEnv);
  let srv = await startServer(enabled);
  try {
    const form = await (await fetch(`${srv.base}/login`)).text();
    assert.match(form, /<form method="POST" action="\/api\/architect\/_gate\/login">/);
    assert.match(form, /Garuda/);

    const err = await (await fetch(`${srv.base}/login?error=1`)).text();
    assert.match(err, /Incorrect password/);
  } finally {
    await srv.close();
  }

  const ended = loadServer({ ...enabledEnv, DEMO_AUTH_EXPIRES_AT: PAST });
  srv = await startServer(ended);
  try {
    const html = await (await fetch(`${srv.base}/login`)).text();
    assert.match(html, /This demo access window has ended\./);
    assert.doesNotMatch(html, /<form/);
  } finally {
    await srv.close();
  }
});

test("the gate is additive — /api/health still works, /api/chat is not auth-gated", async () => {
  const m = loadServer(enabledEnv);
  const { base, close } = await startServer(m);
  try {
    const health = await fetch(`${base}/api/health`);
    assert.equal(health.status, 200);
    assert.deepEqual(await health.json(), { status: "ok", service: "architect-bot" });

    // /api/chat performs its own validation (400 for missing message) and does NOT
    // require the demo_auth cookie — nginx gates it, not this handler.
    const chat = await fetch(`${base}/api/chat`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({}),
    });
    assert.equal(chat.status, 400);
  } finally {
    await close();
  }
});
