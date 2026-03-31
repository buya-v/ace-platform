import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { GatewayClient } from "../gateway-client.js";

export function registerTicketTools(server: McpServer, client: GatewayClient): void {
  server.tool(
    "create_ticket",
    "Create a new support or issue ticket",
    {
      title: z.string().describe("Ticket title / short summary"),
      description: z.string().describe("Detailed description of the issue or request"),
      category: z.string().describe("Ticket category (e.g., trading, compliance, technical, billing)"),
      priority: z.string().describe("Priority level (low, medium, high, critical)"),
    },
    async ({ title, description, category, priority }) => {
      const data = await client.request("POST", "/api/v1/tickets", {
        title,
        description,
        category,
        priority,
      });
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "list_tickets",
    "List tickets with optional filters by status, category, and limit",
    {
      status: z.string().optional().describe("Filter by ticket status (open, in_progress, resolved, closed)"),
      category: z.string().optional().describe("Filter by ticket category"),
      limit: z.number().optional().describe("Maximum number of tickets to return (default 50)"),
    },
    async ({ status, category, limit }) => {
      const params = new URLSearchParams();
      if (status) params.set("status", status);
      if (category) params.set("category", category);
      if (limit) params.set("limit", String(limit));
      const query = params.toString() ? `?${params.toString()}` : "";
      const data = await client.request("GET", `/api/v1/tickets${query}`);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "get_ticket",
    "Get full details of a specific ticket by ID",
    {
      id: z.string().describe("Ticket ID"),
    },
    async ({ id }) => {
      const data = await client.request("GET", `/api/v1/tickets/${id}`);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "update_ticket",
    "Update a ticket's status, assignee, or priority",
    {
      id: z.string().describe("Ticket ID to update"),
      status: z.string().optional().describe("New status (open, in_progress, resolved, closed)"),
      assignee_id: z.string().optional().describe("User ID to assign the ticket to"),
      priority: z.string().optional().describe("New priority (low, medium, high, critical)"),
    },
    async ({ id, status, assignee_id, priority }) => {
      const body: Record<string, string> = {};
      if (status) body.status = status;
      if (assignee_id) body.assignee_id = assignee_id;
      if (priority) body.priority = priority;
      const data = await client.request("PATCH", `/api/v1/tickets/${id}`, body);
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "add_ticket_comment",
    "Add a comment to an existing ticket",
    {
      ticket_id: z.string().describe("Ticket ID to comment on"),
      body: z.string().describe("Comment text"),
    },
    async ({ ticket_id, body: commentBody }) => {
      const data = await client.request("POST", `/api/v1/tickets/${ticket_id}/comments`, {
        body: commentBody,
      });
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );

  server.tool(
    "get_ticket_stats",
    "Get ticket statistics (counts by status, category, average resolution time, etc.)",
    {},
    async () => {
      const data = await client.request("GET", "/api/v1/admin/tickets/stats");
      return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
    },
  );
}
