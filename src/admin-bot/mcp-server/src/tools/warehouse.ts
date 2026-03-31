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

  server.tool(
    "register_facility",
    "Register a new warehouse facility",
    {
      name: z.string().describe("Facility name"),
      address: z.string().optional().describe("Facility address"),
      capacity_mt: z.number().optional().describe("Storage capacity in metric tonnes"),
      commodities_accepted: z.array(z.string()).optional().describe("List of commodity IDs accepted at this facility"),
    },
    async (args) => {
      const data = await client.request("POST", "/api/v1/admin/warehouse/facilities", args);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "issue_receipt",
    "Issue a new warehouse receipt",
    {
      holder_id: z.string().describe("Participant ID of the receipt holder"),
      commodity_id: z.string().describe("Commodity ID (e.g., rice, wheat)"),
      quantity: z.number().describe("Quantity in the commodity's unit of measure"),
      facility_id: z.string().optional().describe("Warehouse facility ID"),
    },
    async (args) => {
      const data = await client.request("POST", "/api/v1/warehouse/receipts", args);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "pledge_receipt",
    "Pledge a warehouse receipt as collateral",
    {
      receipt_id: z.string().describe("Warehouse receipt ID to pledge"),
    },
    async ({ receipt_id }) => {
      const data = await client.request("POST", `/api/v1/warehouse/receipts/${receipt_id}/pledge`, {});
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "create_delivery",
    "Create a delivery instruction for a warehouse receipt",
    {
      receipt_id: z.string().describe("Warehouse receipt ID for delivery"),
      destination: z.string().optional().describe("Delivery destination address or description"),
    },
    async (args) => {
      const data = await client.request("POST", "/api/v1/warehouse/deliveries", args);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );
}
