import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { GatewayClient } from "../gateway-client.js";

export function registerMarketDataTools(server: McpServer, client: GatewayClient): void {
  server.tool(
    "list_instruments",
    "List all available trading instruments",
    {},
    async () => {
      const data = await client.request("GET", "/api/v1/instruments");
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "get_orderbook",
    "Get the current order book for an instrument",
    {
      instrument_id: z.string().describe("Instrument ID (e.g., WHT-HRW-2026M07-UB)"),
      depth: z.number().optional().describe("Order book depth (default 10)"),
    },
    async ({ instrument_id, depth }) => {
      const query = depth ? `?depth=${depth}` : "";
      const data = await client.request("GET", `/api/v1/market-data/${instrument_id}/orderbook${query}`);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "get_ticker",
    "Get the latest ticker data for an instrument",
    {
      instrument_id: z.string().describe("Instrument ID"),
    },
    async ({ instrument_id }) => {
      const data = await client.request("GET", `/api/v1/market-data/${instrument_id}/ticker`);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "get_last_trade",
    "Get the most recent trade for an instrument",
    {
      instrument_id: z.string().describe("Instrument ID"),
    },
    async ({ instrument_id }) => {
      const data = await client.request("GET", `/api/v1/market-data/${instrument_id}/trades/last`);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );
}
