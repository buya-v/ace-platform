import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { GatewayClient } from "../gateway-client.js";

export function registerReportTools(server: McpServer, client: GatewayClient): void {
  server.tool(
    "get_market_summary",
    "Get a summary of market activity including volume, open interest, and price changes",
    {
      instrument_id: z.string().optional().describe("Filter by instrument ID"),
      period: z.string().optional().describe("Time period (today, week, month)"),
    },
    async ({ instrument_id, period }) => {
      const params = new URLSearchParams();
      if (instrument_id) params.set("instrument_id", instrument_id);
      if (period) params.set("period", period);
      const query = params.toString() ? `?${params.toString()}` : "";
      const data = await client.request("GET", `/api/v1/reports/market-summary${query}`);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "get_large_traders",
    "Get large trader positions report for regulatory compliance",
    {
      threshold: z.number().optional().describe("Position threshold for large trader classification"),
    },
    async ({ threshold }) => {
      const query = threshold ? `?threshold=${threshold}` : "";
      const data = await client.request("GET", `/api/v1/reports/large-traders${query}`);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );
}
