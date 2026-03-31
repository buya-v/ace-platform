import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { GatewayClient } from "../gateway-client.js";

export function registerDailyAdminTools(server: McpServer, client: GatewayClient): void {
  server.tool(
    "run_daily_health_check",
    "Run a comprehensive daily health check across all platform services and return a formatted report",
    {},
    async () => {
      let health: unknown;
      try {
        health = await client.request("GET", "/api/v1/admin/health");
      } catch (e) {
        health = { error: String(e) };
      }

      let circuitBreakers: unknown;
      try {
        circuitBreakers = await client.request("GET", "/api/v1/admin/circuit-breakers");
      } catch (e) {
        circuitBreakers = { error: String(e) };
      }

      let haltedInstruments: unknown;
      try {
        haltedInstruments = await client.request("GET", "/api/v1/admin/instruments?status=halted");
      } catch (e) {
        haltedInstruments = { error: String(e) };
      }

      const report = {
        report_type: "daily_health_check",
        generated_at: new Date().toISOString(),
        services: health,
        circuit_breakers: circuitBreakers,
        halted_instruments: haltedInstruments,
      };

      return { content: [{ type: "text" as const, text: JSON.stringify(report, null, 2) }] };
    },
  );

  server.tool(
    "run_eod_report",
    "Generate an end-of-day summary report with settlement cycles, margin stats, and trade activity",
    {},
    async () => {
      let settlementCycles: unknown;
      try {
        settlementCycles = await client.request("GET", "/api/v1/settlement/cycles");
      } catch (e) {
        settlementCycles = { error: String(e) };
      }

      let marginStats: unknown;
      try {
        marginStats = await client.request("GET", "/api/v1/margin/stats");
      } catch (e) {
        marginStats = { error: String(e) };
      }

      let marginCalls: unknown;
      try {
        marginCalls = await client.request("GET", "/api/v1/margin/calls");
      } catch (e) {
        marginCalls = { error: String(e) };
      }

      let recentTrades: unknown;
      try {
        recentTrades = await client.request("GET", "/api/v1/clearing/trades?limit=100");
      } catch (e) {
        recentTrades = { error: String(e) };
      }

      const report = {
        report_type: "end_of_day",
        generated_at: new Date().toISOString(),
        settlement_cycles: settlementCycles,
        margin_stats: marginStats,
        active_margin_calls: marginCalls,
        recent_trades: recentTrades,
      };

      return { content: [{ type: "text" as const, text: JSON.stringify(report, null, 2) }] };
    },
  );

  server.tool(
    "get_aging_tickets",
    "Get open tickets that are older than 24 hours and may need attention",
    {},
    async () => {
      let tickets: unknown;
      try {
        tickets = await client.request("GET", "/api/v1/tickets?status=open");
      } catch (e) {
        return {
          content: [{ type: "text" as const, text: JSON.stringify({ error: String(e) }, null, 2) }],
        };
      }

      const now = Date.now();
      const twentyFourHours = 24 * 60 * 60 * 1000;

      // Filter tickets older than 24h
      let agingTickets: unknown[] = [];
      if (Array.isArray(tickets)) {
        agingTickets = tickets.filter((t: Record<string, unknown>) => {
          const created = t.created_at ?? t.createdAt;
          if (typeof created === "string") {
            return now - new Date(created).getTime() > twentyFourHours;
          }
          return false;
        });
      }

      const result = {
        report_type: "aging_tickets",
        generated_at: new Date().toISOString(),
        threshold_hours: 24,
        total_open: Array.isArray(tickets) ? tickets.length : 0,
        aging_count: agingTickets.length,
        aging_tickets: agingTickets,
      };

      return { content: [{ type: "text" as const, text: JSON.stringify(result, null, 2) }] };
    },
  );

  server.tool(
    "get_unresolved_alerts_summary",
    "Get a summary of unresolved compliance alerts grouped by severity",
    {},
    async () => {
      let alerts: unknown;
      try {
        alerts = await client.request("GET", "/api/v1/compliance/alerts?status=open");
      } catch (e) {
        return {
          content: [{ type: "text" as const, text: JSON.stringify({ error: String(e) }, null, 2) }],
        };
      }

      // Group by severity
      const bySeverity: Record<string, unknown[]> = {};
      let total = 0;

      if (Array.isArray(alerts)) {
        total = alerts.length;
        for (const alert of alerts) {
          const severity = (alert as Record<string, unknown>).severity as string ?? "unknown";
          if (!bySeverity[severity]) {
            bySeverity[severity] = [];
          }
          bySeverity[severity].push(alert);
        }
      }

      const counts: Record<string, number> = {};
      for (const [severity, items] of Object.entries(bySeverity)) {
        counts[severity] = items.length;
      }

      const result = {
        report_type: "unresolved_alerts_summary",
        generated_at: new Date().toISOString(),
        total_unresolved: total,
        by_severity: counts,
        alerts: bySeverity,
      };

      return { content: [{ type: "text" as const, text: JSON.stringify(result, null, 2) }] };
    },
  );
}
