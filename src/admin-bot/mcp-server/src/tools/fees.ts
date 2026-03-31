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
}
