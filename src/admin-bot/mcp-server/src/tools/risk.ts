import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { GatewayClient } from "../gateway-client.js";

export function registerRiskTools(server: McpServer, client: GatewayClient): void {
  server.tool(
    "get_order_limits",
    "Get order size and position limits configuration",
    {
      instrument_id: z.string().optional().describe("Filter by instrument ID"),
    },
    async ({ instrument_id }) => {
      const query = instrument_id ? `?instrument_id=${instrument_id}` : "";
      const data = await client.request("GET", `/api/v1/risk/limits${query}`);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );
}
