#!/usr/bin/env node

/**
 * GarudaX Admin MCP Server
 *
 * Exposes the GarudaX commodity exchange gateway admin APIs as MCP tools
 * that Claude CLI can call directly.
 *
 * Configuration via environment variables:
 *   GARUDAX_GATEWAY_URL   — Gateway base URL (default: http://127.0.0.1:8080)
 *   GARUDAX_ADMIN_EMAIL   — Admin email for JWT authentication
 *   GARUDAX_ADMIN_PASSWORD — Admin password
 */

import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { GatewayClient } from "./gateway-client.js";

// Tool registrations
import { registerHealthTools } from "./tools/health.js";
import { registerTradingTools } from "./tools/trading.js";
import { registerMarginTools } from "./tools/margin.js";
import { registerClearingTools } from "./tools/clearing.js";
import { registerSettlementTools } from "./tools/settlement.js";
import { registerComplianceTools } from "./tools/compliance.js";
import { registerParticipantTools } from "./tools/participants.js";
import { registerMarketDataTools } from "./tools/market_data.js";
import { registerWarehouseTools } from "./tools/warehouse.js";
import { registerRiskTools } from "./tools/risk.js";
import { registerReportTools } from "./tools/reports.js";
import { registerFeeTools } from "./tools/fees.js";
import { registerTicketTools } from "./tools/tickets.js";
import { registerDailyAdminTools } from "./tools/daily_admin.js";
import { registerSecuritiesTools } from "./tools/securities.js";

// Resource registrations
import { registerSystemStatusResource } from "./resources/system_status.js";
import { registerActiveAlertsResource } from "./resources/active_alerts.js";
import { registerTicketResources } from "./resources/tickets.js";

async function main(): Promise<void> {
  const gatewayUrl = process.env.GARUDAX_GATEWAY_URL ?? "http://127.0.0.1:8080";
  const adminEmail = process.env.GARUDAX_ADMIN_EMAIL;
  const adminPassword = process.env.GARUDAX_ADMIN_PASSWORD;

  const client = new GatewayClient({
    baseUrl: gatewayUrl,
    email: adminEmail,
    password: adminPassword,
  });

  const server = new McpServer({
    name: "garudax-admin",
    version: "1.0.0",
  });

  // Register all tools
  registerHealthTools(server, client);
  registerTradingTools(server, client);
  registerMarginTools(server, client);
  registerClearingTools(server, client);
  registerSettlementTools(server, client);
  registerComplianceTools(server, client);
  registerParticipantTools(server, client);
  registerMarketDataTools(server, client);
  registerWarehouseTools(server, client);
  registerRiskTools(server, client);
  registerReportTools(server, client);
  registerFeeTools(server, client);
  registerTicketTools(server, client);
  registerDailyAdminTools(server, client);
  registerSecuritiesTools(server, client);

  // Register resources
  registerSystemStatusResource(server, client);
  registerActiveAlertsResource(server, client);
  registerTicketResources(server, client);

  // Connect via stdio transport
  const transport = new StdioServerTransport();
  await server.connect(transport);
}

main().catch((err) => {
  console.error("Fatal error:", err);
  process.exit(1);
});
