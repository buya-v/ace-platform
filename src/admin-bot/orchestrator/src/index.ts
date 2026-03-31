import { createAPIServer } from './api.js';
import { DailyScheduler } from './daily-admin.js';

const PORT = parseInt(process.env.PORT ?? '3100', 10);

const server = createAPIServer();
const scheduler = new DailyScheduler();

server.listen(PORT, () => {
  console.log(`[bot-orchestrator] Listening on port ${PORT}`);
  console.log(`[bot-orchestrator] Endpoints:`);
  console.log(`  POST /chat         — Route message through nano/claude`);
  console.log(`  GET  /suggestions  — Page-aware quick actions`);
  console.log(`  GET  /health       — Orchestrator health check`);
});

scheduler.start();

// Graceful shutdown
function shutdown(signal: string): void {
  console.log(`[bot-orchestrator] Received ${signal}, shutting down...`);
  scheduler.stop();
  server.close(() => {
    console.log('[bot-orchestrator] Server closed');
    process.exit(0);
  });

  // Force exit after 10 seconds
  setTimeout(() => {
    console.error('[bot-orchestrator] Forced shutdown after timeout');
    process.exit(1);
  }, 10_000).unref();
}

process.on('SIGTERM', () => shutdown('SIGTERM'));
process.on('SIGINT', () => shutdown('SIGINT'));
