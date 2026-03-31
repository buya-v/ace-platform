import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { GatewayClient } from "../gateway-client.js";

export function registerTicketResources(server: McpServer, client: GatewayClient): void {
  server.resource(
    "open-tickets",
    "garudax://tickets/open",
    {
      description: "List of currently open support and issue tickets",
      mimeType: "application/json",
    },
    async () => {
      let tickets: unknown = { error: "Unable to fetch tickets" };

      try {
        tickets = await client.request("GET", "/api/v1/tickets?status=open");
      } catch (e) {
        tickets = { error: String(e) };
      }

      const result = {
        timestamp: new Date().toISOString(),
        tickets,
      };

      return {
        contents: [
          {
            uri: "garudax://tickets/open",
            mimeType: "application/json",
            text: JSON.stringify(result, null, 2),
          },
        ],
      };
    },
  );

  server.resource(
    "ticket-stats",
    "garudax://tickets/stats",
    {
      description: "Ticket statistics including counts by status, category, and resolution metrics",
      mimeType: "application/json",
    },
    async () => {
      let stats: unknown = { error: "Unable to fetch ticket stats" };

      try {
        stats = await client.request("GET", "/api/v1/admin/tickets/stats");
      } catch (e) {
        stats = { error: String(e) };
      }

      const result = {
        timestamp: new Date().toISOString(),
        stats,
      };

      return {
        contents: [
          {
            uri: "garudax://tickets/stats",
            mimeType: "application/json",
            text: JSON.stringify(result, null, 2),
          },
        ],
      };
    },
  );
}
