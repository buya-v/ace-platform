import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { GatewayClient } from "../gateway-client.js";

export function registerTradingTools(server: McpServer, client: GatewayClient): void {
  server.tool(
    "halt_instrument",
    "Halt trading on a specific instrument",
    {
      instrument_id: z.string().describe("Instrument ID (e.g., WHT-HRW-2026M07-UB)"),
      reason: z.string().describe("Reason for halting trading"),
    },
    async ({ instrument_id, reason }) => {
      const data = await client.request("POST", `/api/v1/admin/instruments/${instrument_id}/halt`, { reason });
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "resume_instrument",
    "Resume trading on a previously halted instrument",
    {
      instrument_id: z.string().describe("Instrument ID to resume"),
    },
    async ({ instrument_id }) => {
      const data = await client.request("POST", `/api/v1/admin/instruments/${instrument_id}/resume`);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "get_circuit_breakers",
    "Get current circuit breaker status for all instruments",
    {},
    async () => {
      const data = await client.request("GET", "/api/v1/admin/circuit-breakers");
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "mass_cancel",
    "Mass cancel all open orders for a participant or instrument",
    {
      participant_id: z.string().optional().describe("Participant ID to cancel orders for"),
      instrument_id: z.string().optional().describe("Instrument ID to cancel orders for"),
      reason: z.string().describe("Reason for mass cancellation"),
    },
    async ({ participant_id, instrument_id, reason }) => {
      const body: Record<string, string> = { reason };
      if (participant_id) body.participant_id = participant_id;
      if (instrument_id) body.instrument_id = instrument_id;
      const data = await client.request("POST", "/api/v1/admin/orders/mass-cancel", body);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );
}
