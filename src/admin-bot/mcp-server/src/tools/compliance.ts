import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { GatewayClient } from "../gateway-client.js";

export function registerComplianceTools(server: McpServer, client: GatewayClient): void {
  server.tool(
    "get_compliance_alerts",
    "Get compliance alerts, optionally filtered by status or type",
    {
      status: z.string().optional().describe("Filter by alert status (open, resolved, escalated)"),
      alert_type: z.string().optional().describe("Filter by alert type (suspicious_activity, large_position, etc.)"),
    },
    async ({ status, alert_type }) => {
      const params = new URLSearchParams();
      if (status) params.set("status", status);
      if (alert_type) params.set("type", alert_type);
      const query = params.toString() ? `?${params.toString()}` : "";
      const data = await client.request("GET", `/api/v1/compliance/alerts${query}`);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "resolve_alert",
    "Resolve a compliance alert with a resolution note",
    {
      alert_id: z.string().describe("Alert ID to resolve"),
      resolution: z.string().describe("Resolution note explaining the action taken"),
    },
    async ({ alert_id, resolution }) => {
      const data = await client.request("POST", `/api/v1/compliance/alerts/${alert_id}/resolve`, { resolution });
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "get_audit_trail",
    "Get audit trail entries for compliance review",
    {
      participant_id: z.string().optional().describe("Filter by participant ID"),
      action: z.string().optional().describe("Filter by action type"),
      limit: z.number().optional().describe("Maximum number of entries to return (default 100)"),
    },
    async ({ participant_id, action, limit }) => {
      const params = new URLSearchParams();
      if (participant_id) params.set("participant_id", participant_id);
      if (action) params.set("action", action);
      if (limit) params.set("limit", String(limit));
      const query = params.toString() ? `?${params.toString()}` : "";
      const data = await client.request("GET", `/api/v1/compliance/audit${query}`);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );
}
