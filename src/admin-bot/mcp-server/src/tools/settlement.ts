import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { GatewayClient } from "../gateway-client.js";

export function registerSettlementTools(server: McpServer, client: GatewayClient): void {
  server.tool(
    "get_settlement_cycles",
    "Get settlement cycles, optionally filtered by status",
    {
      status: z.string().optional().describe("Filter by cycle status (pending, in_progress, completed, failed)"),
    },
    async ({ status }) => {
      const query = status ? `?status=${status}` : "";
      const data = await client.request("GET", `/api/v1/settlement/cycles${query}`);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );
}
