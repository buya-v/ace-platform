import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { GatewayClient } from "../gateway-client.js";

export function registerMarginTools(server: McpServer, client: GatewayClient): void {
  server.tool(
    "get_margin_calls",
    "Get all active margin calls",
    {
      participant_id: z.string().optional().describe("Filter by participant ID"),
    },
    async ({ participant_id }) => {
      const query = participant_id ? `?participant_id=${participant_id}` : "";
      const data = await client.request("GET", `/api/v1/margin/calls${query}`);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "get_margin_stats",
    "Get margin utilization statistics",
    {},
    async () => {
      const data = await client.request("GET", "/api/v1/margin/stats");
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "trigger_margin_calculation",
    "Trigger a margin recalculation for a participant",
    {
      participant_id: z.string().describe("Participant ID to recalculate margin for"),
    },
    async ({ participant_id }) => {
      const data = await client.request("POST", `/api/v1/margin/calculate`, { participant_id });
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );
}
