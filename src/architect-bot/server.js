const express = require("express");
const cors = require("cors");
const fs = require("fs");
const path = require("path");

const PORT = process.env.PORT || 3010;

// DeepSeek is OpenAI-compatible. Keys may be comma-separated for failover.
const DEEPSEEK_API_KEYS = (process.env.DEEPSEEK_API_KEY || "").split(",").filter(Boolean);
const DEEPSEEK_BASE_URL = process.env.DEEPSEEK_BASE_URL || "https://api.deepseek.com";

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

app.listen(PORT, () => {
  console.log(`architect-bot listening on port ${PORT}`);
});
