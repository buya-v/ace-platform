import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { GatewayClient } from "../gateway-client.js";

export function registerClearingTools(server: McpServer, client: GatewayClient): void {
  server.tool(
    "get_positions",
    "Get clearing positions, optionally filtered by participant",
    {
      participant_id: z.string().optional().describe("Filter by participant ID"),
      instrument_id: z.string().optional().describe("Filter by instrument ID"),
    },
    async ({ participant_id, instrument_id }) => {
      const params = new URLSearchParams();
      if (participant_id) params.set("participant_id", participant_id);
      if (instrument_id) params.set("instrument_id", instrument_id);
      const query = params.toString() ? `?${params.toString()}` : "";
      const data = await client.request("GET", `/api/v1/clearing/positions${query}`);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "get_netting",
    "Get netting results for settlement",
    {
      cycle_id: z.string().optional().describe("Settlement cycle ID"),
    },
    async ({ cycle_id }) => {
      const query = cycle_id ? `?cycle_id=${cycle_id}` : "";
      const data = await client.request("GET", `/api/v1/clearing/netting${query}`);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );
}
