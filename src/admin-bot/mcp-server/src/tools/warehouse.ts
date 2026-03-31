import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { GatewayClient } from "../gateway-client.js";

export function registerWarehouseTools(server: McpServer, client: GatewayClient): void {
  server.tool(
    "get_inventory",
    "Get warehouse inventory levels",
    {
      commodity: z.string().optional().describe("Filter by commodity type"),
      warehouse_id: z.string().optional().describe("Filter by warehouse ID"),
    },
    async ({ commodity, warehouse_id }) => {
      const params = new URLSearchParams();
      if (commodity) params.set("commodity", commodity);
      if (warehouse_id) params.set("warehouse_id", warehouse_id);
      const query = params.toString() ? `?${params.toString()}` : "";
      const data = await client.request("GET", `/api/v1/warehouse/inventory${query}`);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "get_receipts",
    "Get warehouse receipts",
    {
      participant_id: z.string().optional().describe("Filter by participant ID"),
      status: z.string().optional().describe("Filter by receipt status (active, cancelled, delivered)"),
    },
    async ({ participant_id, status }) => {
      const params = new URLSearchParams();
      if (participant_id) params.set("participant_id", participant_id);
      if (status) params.set("status", status);
      const query = params.toString() ? `?${params.toString()}` : "";
      const data = await client.request("GET", `/api/v1/warehouse/receipts${query}`);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );
}
