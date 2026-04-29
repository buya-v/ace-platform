const express = require("express");
const cors = require("cors");
const fs = require("fs");
const path = require("path");
const { GoogleGenerativeAI } = require("@google/generative-ai");

const PORT = process.env.PORT || 3010;
const GEMINI_API_KEYS = (process.env.GEMINI_API_KEY || "").split(",").filter(Boolean);
const GEMINI_API_KEY = GEMINI_API_KEYS[0];

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

// ─── System prompt ─────────────────────────────────────────────────────────────

const SYSTEM_PROMPT = `You are the Chief Architect of GarudaX, a multi-tenant AI-native securities exchange platform built to replace MillenniumIT at the Mongolian Stock Exchange (MSE).

You are presenting GarudaX to the MSE board, FRC regulators, and technical evaluation team.

Your persona:
- Confident and authoritative — you built this platform
- Technically deep — you know the codebase (65 types, 48 stores, 94 handlers, 2540+ tests)
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

// ─── Gemini client ─────────────────────────────────────────────────────────────

const genAI = new GoogleGenerativeAI(GEMINI_API_KEY);

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

  // Try multiple API keys and models in order of preference
  const modelNames = ["gemini-2.5-flash-lite", "gemini-2.0-flash-lite", "gemini-2.5-flash", "gemini-2.0-flash-001"];
  let lastError = null;

  for (const apiKey of GEMINI_API_KEYS) {
    const ai = new GoogleGenerativeAI(apiKey);
    for (const modelName of modelNames) {
    try {
      const model = ai.getGenerativeModel({
        model: modelName,
        systemInstruction: SYSTEM_PROMPT,
        generationConfig: {
          temperature: 0.7,
          maxOutputTokens: 4096,
        },
      });

      const geminiHistory = history.map((entry) => ({
        role: entry.role === "assistant" ? "model" : "user",
        parts: [{ text: entry.content }],
      }));

      const chat = model.startChat({ history: geminiHistory });
      const result = await chat.sendMessage(message);
      const response = result.response.text();

      console.log(`Response from ${modelName} (${response.length} chars)`);
      res.json({ response });
      return;
    } catch (err) {
      console.error(`key=${apiKey.slice(-6)} ${modelName} error:`, err?.message || err);
      lastError = err;
    }
  }
  }

  // All keys and models failed
  console.error("All Gemini models failed:", lastError?.message);
  {
    res.json({
      response:
        "I apologize, I'm having trouble connecting to my AI backend. All models are currently unavailable. Please try again in a moment.",
      error: true,
    });
  }
});

// ─── Start ─────────────────────────────────────────────────────────────────────

app.listen(PORT, () => {
  console.log(`architect-bot listening on port ${PORT}`);
});
