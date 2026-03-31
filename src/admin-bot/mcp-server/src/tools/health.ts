import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { GatewayClient } from "../gateway-client.js";

export function registerHealthTools(server: McpServer, client: GatewayClient): void {
  server.tool(
    "get_system_health",
    "Get health status of all GarudaX platform services",
    {},
    async () => {
      const data = await client.request("GET", "/api/v1/admin/health");
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );
}
