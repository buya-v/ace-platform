# T2 Handoff — Architect Bot Express.js Backend

## Summary

Created the `architect-bot` Express.js backend service that serves as a Gemini-powered AI persona for GarudaX pitch presentations to the MSE board, FRC regulators, and technical evaluation team.

## Deliverables

### Files Created
- `src/architect-bot/package.json` — Node.js package, dependencies: express ^4.21.0, @google/generative-ai ^0.21.0, cors ^2.8.5
- `src/architect-bot/server.js` — Express server with startup knowledge loading, `/api/health` GET, `/api/chat` POST with Gemini multi-turn conversation support
- `src/architect-bot/Dockerfile` — node:22-alpine, production npm install, exposes 3010

### Files Modified
- `docker-compose.yml` — Added `architect-bot` service before the Reverse Proxy section, port 3010:3010, `restart: unless-stopped`, healthcheck via wget

## Decisions Made

1. **Model updated**: Spec called for `gemini-2.0-flash` which is no longer available (API returns 404). Updated to `gemini-2.5-flash` — confirmed working via live API test.

2. **History role mapping**: Incoming history with `role: "assistant"` is mapped to `"model"` for the Gemini SDK, `role: "user"` passes through unchanged. Required by Gemini `startChat()` API.

3. **System instruction approach**: Used `systemInstruction` field on `getGenerativeModel()` rather than prepending to the first user message — correct Gemini SDK pattern for persistent persona across turns.

4. **Knowledge loading**: Files are read synchronously at startup before `app.listen()`, sorted alphabetically, concatenated with `---` section headers. Server does not start until all knowledge is loaded — no race condition.

## Test Results

```
Loaded 5 knowledge files: competitive-landscape.md, garudax-overview.md,
  millenniumit-comparison.md, mse-context.md, securities-domain.md
architect-bot listening on port 3010

GET /api/health  → {"status":"ok","service":"architect-bot"}

POST /api/chat {"message":"What is GarudaX in one sentence?"}
→ {"response":"GarudaX is a multi-tenant, AI-native operating platform that
   hosts regulated trading venues, offering a modern, open-stack alternative
   to legacy systems like MillenniumIT."}
```

72 packages installed, 0 vulnerabilities.

## Knowledge Base Stats

| File | Size |
|---|---|
| competitive-landscape.md | 6,027 bytes |
| garudax-overview.md | 5,087 bytes |
| millenniumit-comparison.md | 6,930 bytes |
| mse-context.md | 5,580 bytes |
| securities-domain.md | 8,811 bytes |
| **Total** | **~32.4 KB** |

## Blockers Found

None.

## Suggested Follow-ups

- **Frontend chat UI**: Wire up `POST /api/chat` with streaming display if needed. The API already supports `history[]` for multi-turn context.
- **Docker network**: `architect-bot` is on `garudax-network` with no `depends_on` — fully independent, starts without any other service.
- **Model pinning**: Consider pinning to a specific gemini-2.5-flash version (e.g. `gemini-2.5-flash-preview-04-17`) for reproducibility.
- **Rate limiting**: No rate limiting on `/api/chat` — add if exposing publicly during the MSE demo.
