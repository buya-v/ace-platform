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

  server.tool(
    "create_instrument",
    "Create a new tradable instrument",
    {
      id: z.string().describe("Instrument ID (e.g., RIC-2027M07-UB)"),
      commodity_id: z.string().describe("Commodity ID (e.g., rice)"),
      name: z.string().describe("Display name"),
      delivery_month: z.number().describe("Delivery month (1-12)"),
      delivery_year: z.number().describe("Delivery year"),
      contract_size: z.number().describe("Contract size"),
      tick_size: z.number().describe("Minimum price increment"),
      currency: z.string().optional().describe("Currency code (default: MNT)"),
      settlement_type: z.string().describe("PHYSICAL or CASH"),
    },
    async (args) => {
      const data = await client.request("POST", "/api/v1/admin/instruments", args);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "update_instrument",
    "Update an existing instrument's status or trading hours",
    {
      id: z.string().describe("Instrument ID to update"),
      status: z.string().optional().describe("New status (e.g., active, halted, closed)"),
      trading_hours: z.string().optional().describe("Trading hours specification (e.g., '09:00-15:00')"),
    },
    async ({ id, ...rest }) => {
      const data = await client.request("PUT", `/api/v1/admin/instruments/${id}`, rest);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "create_commodity",
    "Create a new commodity",
    {
      id: z.string().describe("Commodity ID (e.g., rice, wheat)"),
      name: z.string().describe("Display name"),
      category: z.string().describe("Commodity category (e.g., grains, metals)"),
      unit: z.string().describe("Unit of measure (e.g., MT, BBL)"),
    },
    async (args) => {
      const data = await client.request("POST", "/api/v1/admin/commodities", args);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );
}
