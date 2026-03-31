import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { GatewayClient } from "../gateway-client.js";

export function registerParticipantTools(server: McpServer, client: GatewayClient): void {
  server.tool(
    "list_participants",
    "List all exchange participants with their KYC status",
    {
      status: z.string().optional().describe("Filter by KYC status (pending, approved, rejected)"),
    },
    async ({ status }) => {
      const query = status ? `?status=${status}` : "";
      const data = await client.request("GET", `/api/v1/participants${query}`);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "approve_kyc",
    "Approve a participant's KYC application",
    {
      participant_id: z.string().describe("Participant ID to approve"),
      notes: z.string().optional().describe("Approval notes"),
    },
    async ({ participant_id, notes }) => {
      const body: Record<string, string> = {};
      if (notes) body.notes = notes;
      const data = await client.request("POST", `/api/v1/participants/${participant_id}/kyc/approve`, body);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "reject_kyc",
    "Reject a participant's KYC application",
    {
      participant_id: z.string().describe("Participant ID to reject"),
      reason: z.string().describe("Reason for rejection"),
    },
    async ({ participant_id, reason }) => {
      const data = await client.request("POST", `/api/v1/participants/${participant_id}/kyc/reject`, { reason });
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "screen_participant",
    "Run a compliance screening check on a participant",
    {
      participant_id: z.string().describe("Participant ID to screen"),
    },
    async ({ participant_id }) => {
      const data = await client.request("POST", "/api/v1/screening/check", { participant_id });
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "batch_screen",
    "Run a batch compliance screening check on all participants",
    {},
    async () => {
      const data = await client.request("POST", "/api/v1/screening/batch", {});
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );
}
