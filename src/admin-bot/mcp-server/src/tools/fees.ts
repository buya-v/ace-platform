import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { GatewayClient } from "../gateway-client.js";

export function registerFeeTools(server: McpServer, client: GatewayClient): void {
  server.tool(
    "get_fee_schedule",
    "Get the current fee schedule for trading, clearing, and settlement",
    {
      instrument_id: z.string().optional().describe("Filter by instrument ID"),
      fee_type: z.string().optional().describe("Filter by fee type (trading, clearing, settlement, delivery)"),
    },
    async ({ instrument_id, fee_type }) => {
      const params = new URLSearchParams();
      if (instrument_id) params.set("instrument_id", instrument_id);
      if (fee_type) params.set("type", fee_type);
      const query = params.toString() ? `?${params.toString()}` : "";
      const data = await client.request("GET", `/api/v1/fees/schedule${query}`);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "create_fee_schedule",
    "Create a new fee schedule",
    {
      name: z.string().describe("Fee schedule name"),
      effective_from: z.string().describe("Effective date (ISO 8601 format, e.g., 2026-04-01T00:00:00Z)"),
    },
    async (args) => {
      const data = await client.request("POST", "/api/v1/admin/fees/schedules", args);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "create_fee_rule",
    "Create a new fee rule within a fee schedule",
    {
      schedule_id: z.string().describe("Fee schedule ID"),
      fee_type: z.string().describe("Fee type (trading, clearing, settlement, delivery)"),
      participant_tier: z.string().describe("Participant tier (e.g., tier1, tier2, tier3)"),
      rate_bps: z.number().describe("Fee rate in basis points"),
      min_fee: z.number().optional().describe("Minimum fee amount"),
      max_fee: z.number().optional().describe("Maximum fee amount"),
    },
    async (args) => {
      const data = await client.request("POST", "/api/v1/admin/fees/rules", args);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "set_participant_tier",
    "Set the fee tier for a participant",
    {
      participant_id: z.string().describe("Participant ID"),
      tier: z.string().describe("Fee tier to assign (e.g., tier1, tier2, tier3)"),
    },
    async ({ participant_id, tier }) => {
      const data = await client.request("PUT", `/api/v1/admin/fees/tiers/${participant_id}`, { tier });
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );
}
