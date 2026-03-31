import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { GatewayClient } from "../gateway-client.js";

export function registerSystemStatusResource(server: McpServer, client: GatewayClient): void {
  server.resource(
    "system-status",
    "garudax://system/status",
    {
      description: "Current GarudaX platform health status and halted instruments",
      mimeType: "application/json",
    },
    async () => {
      let health: unknown = { error: "Unable to fetch health" };
      let haltedInstruments: unknown = { error: "Unable to fetch halted instruments" };

      try {
        health = await client.request("GET", "/api/v1/admin/health");
      } catch (e) {
        health = { error: String(e) };
      }

      try {
        haltedInstruments = await client.request("GET", "/api/v1/admin/instruments?status=halted");
      } catch (e) {
        haltedInstruments = { error: String(e) };
      }

      const status = {
        timestamp: new Date().toISOString(),
        health,
        halted_instruments: haltedInstruments,
      };

      return {
        contents: [
          {
            uri: "garudax://system/status",
            mimeType: "application/json",
            text: JSON.stringify(status, null, 2),
          },
        ],
      };
    },
  );
}
