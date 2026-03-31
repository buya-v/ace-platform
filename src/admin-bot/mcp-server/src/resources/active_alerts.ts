import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { GatewayClient } from "../gateway-client.js";

export function registerActiveAlertsResource(server: McpServer, client: GatewayClient): void {
  server.resource(
    "active-alerts",
    "garudax://compliance/active-alerts",
    {
      description: "Unresolved compliance alerts requiring attention",
      mimeType: "application/json",
    },
    async () => {
      let alerts: unknown = { error: "Unable to fetch alerts" };

      try {
        alerts = await client.request("GET", "/api/v1/compliance/alerts?status=open");
      } catch (e) {
        alerts = { error: String(e) };
      }

      const result = {
        timestamp: new Date().toISOString(),
        alerts,
      };

      return {
        contents: [
          {
            uri: "garudax://compliance/active-alerts",
            mimeType: "application/json",
            text: JSON.stringify(result, null, 2),
          },
        ],
      };
    },
  );
}
