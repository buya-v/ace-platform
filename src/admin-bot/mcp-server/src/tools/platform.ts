import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { GatewayClient } from "../gateway-client.js";

export function registerPlatformTools(server: McpServer, client: GatewayClient): void {
  // ── 1. List tenants ───────────────────────────────────────────────────────
  server.tool(
    "list_tenants",
    "List all tenants registered on the GarudaX platform",
    {},
    async () => {
      const data = await client.request("GET", "/platform/v1/tenants");
      const items = Array.isArray(data) ? data : (data as Record<string, unknown[]>).tenants ?? [];
      if (items.length === 0) {
        return { content: [{ type: "text" as const, text: "No tenants found." }] };
      }

      const header = "ID".padEnd(24) + "NAME".padEnd(24) + "STATUS".padEnd(14) + "FLAGSHIP".padEnd(10) + "TIER";
      const divider = "─".repeat(header.length);
      const lines = (items as Record<string, unknown>[]).map((t) => {
        const id = String(t.id ?? "").padEnd(24);
        const name = String(t.name ?? "").padEnd(24).slice(0, 24);
        const status = String(t.status ?? "").padEnd(14);
        const flagship = String(t.is_flagship ?? t.flagship ?? false).padEnd(10);
        const tier = String(t.governance_tier ?? t.tier ?? "");
        return `${id}${name}${status}${flagship}${tier}`;
      });

      const text = [header, divider, ...lines].join("\n");
      return { content: [{ type: "text" as const, text }] };
    },
  );

  // ── 2. Get a single tenant ────────────────────────────────────────────────
  server.tool(
    "get_tenant",
    "Get details of a specific tenant by ID",
    {
      tenant_id: z.string().describe("Tenant ID (slug, e.g. ace-commodities)"),
    },
    async ({ tenant_id }) => {
      const data = await client.request("GET", `/platform/v1/tenants/${tenant_id}`);
      const t = data as Record<string, unknown>;
      const lines = [
        `ID               : ${t.id ?? tenant_id}`,
        `Name             : ${t.name ?? "—"}`,
        `Status           : ${t.status ?? "—"}`,
        `Flagship         : ${t.is_flagship ?? t.flagship ?? false}`,
        `Governance Tier  : ${t.governance_tier ?? t.tier ?? "—"}`,
        `Created At       : ${t.created_at ?? "—"}`,
        `Updated At       : ${t.updated_at ?? "—"}`,
      ];
      return { content: [{ type: "text" as const, text: lines.join("\n") }] };
    },
  );

  // ── 3. Create a tenant ────────────────────────────────────────────────────
  server.tool(
    "create_tenant",
    "Create a new tenant on the GarudaX platform and provision its infrastructure",
    {
      id: z.string().describe("Tenant slug ID (e.g. mse-equities)"),
      name: z.string().describe("Human-readable tenant name"),
      governance_tier: z
        .string()
        .optional()
        .default("STANDARD")
        .describe("Governance tier: STANDARD, PREMIUM, or FLAGSHIP (default: STANDARD)"),
    },
    async ({ id, name, governance_tier }) => {
      const body = { id, name, governance_tier: governance_tier ?? "STANDARD" };
      const data = await client.request("POST", "/platform/v1/tenants", body);
      const r = data as Record<string, unknown>;
      const tenant = (r.tenant ?? r) as Record<string, unknown>;
      const provisioning = r.provisioning as Record<string, unknown> | undefined;

      const lines = [
        `Tenant created successfully.`,
        ``,
        `ID               : ${tenant.id ?? id}`,
        `Name             : ${tenant.name ?? name}`,
        `Status           : ${tenant.status ?? "ONBOARDING"}`,
        `Governance Tier  : ${tenant.governance_tier ?? governance_tier ?? "STANDARD"}`,
      ];

      if (provisioning) {
        lines.push(``, `Provisioning Result:`);
        const schemas = Array.isArray(provisioning.schemas)
          ? (provisioning.schemas as unknown[]).join(", ")
          : String(provisioning.schemas ?? "—");
        const topics = Array.isArray(provisioning.topics)
          ? (provisioning.topics as unknown[]).join(", ")
          : String(provisioning.topics ?? "—");
        lines.push(`  Schemas  : ${schemas}`);
        lines.push(`  Topics   : ${topics}`);
        lines.push(`  Status   : ${provisioning.status ?? "—"}`);
      }

      return { content: [{ type: "text" as const, text: lines.join("\n") }] };
    },
  );

  // ── 4. Update tenant status ───────────────────────────────────────────────
  server.tool(
    "update_tenant_status",
    "Update the lifecycle status of a tenant (ACTIVE, SUSPENDED, or ONBOARDING)",
    {
      tenant_id: z.string().describe("Tenant ID to update"),
      status: z
        .enum(["ACTIVE", "SUSPENDED", "ONBOARDING"])
        .describe("New status: ACTIVE, SUSPENDED, or ONBOARDING"),
    },
    async ({ tenant_id, status }) => {
      const data = await client.request("PUT", `/platform/v1/tenants/${tenant_id}/status`, { status });
      const t = data as Record<string, unknown>;
      const lines = [
        `Tenant status updated.`,
        ``,
        `ID         : ${t.id ?? tenant_id}`,
        `New Status : ${t.status ?? status}`,
        `Updated At : ${t.updated_at ?? "—"}`,
      ];
      return { content: [{ type: "text" as const, text: lines.join("\n") }] };
    },
  );

  // ── 5. Get tenant config ──────────────────────────────────────────────────
  server.tool(
    "get_tenant_config",
    "Get the runtime configuration for a tenant including trading hours, settlement, circuit breakers, and features",
    {
      tenant_id: z.string().describe("Tenant ID to fetch config for"),
    },
    async ({ tenant_id }) => {
      const data = await client.request("GET", `/platform/v1/tenants/${tenant_id}/config`);
      const cfg = data as Record<string, unknown>;

      const lines: string[] = [`Tenant Config: ${tenant_id}`, "─".repeat(50)];

      // Trading hours
      const hours = cfg.trading_hours as Record<string, unknown> | undefined;
      if (hours) {
        lines.push(``, `Trading Hours:`);
        lines.push(`  Open  : ${hours.open ?? "—"}`);
        lines.push(`  Close : ${hours.close ?? "—"}`);
        lines.push(`  Zone  : ${hours.timezone ?? hours.zone ?? "—"}`);
      }

      // Settlement
      const settlement = cfg.settlement as Record<string, unknown> | undefined;
      if (settlement) {
        lines.push(``, `Settlement:`);
        lines.push(`  Cycle    : ${settlement.cycle ?? "—"}`);
        lines.push(`  Currency : ${settlement.currency ?? "—"}`);
        lines.push(`  Cutoff   : ${settlement.cutoff ?? "—"}`);
      }

      // Circuit breakers
      const breakers = cfg.circuit_breakers as Record<string, unknown> | undefined;
      if (breakers) {
        lines.push(``, `Circuit Breakers:`);
        for (const [key, val] of Object.entries(breakers)) {
          lines.push(`  ${key.padEnd(16)} : ${val}`);
        }
      }

      // Features
      const features = cfg.features as Record<string, unknown> | undefined;
      if (features) {
        lines.push(``, `Features:`);
        for (const [key, val] of Object.entries(features)) {
          lines.push(`  ${key.padEnd(24)} : ${val}`);
        }
      }

      // Fallback: show raw config if none of the known sections present
      if (!hours && !settlement && !breakers && !features) {
        lines.push(``, JSON.stringify(cfg, null, 2));
      }

      return { content: [{ type: "text" as const, text: lines.join("\n") }] };
    },
  );

  // ── 6. Provision tenant ───────────────────────────────────────────────────
  server.tool(
    "provision_tenant",
    "Provision a new tenant on the GarudaX platform: creates the tenant record and initialises database schemas, Kafka topics, and service configuration",
    {
      id: z.string().describe("Tenant slug ID (e.g. mse-equities)"),
      name: z.string().describe("Human-readable tenant name"),
    },
    async ({ id, name }) => {
      const data = await client.request("POST", "/platform/v1/tenants", { id, name, governance_tier: "STANDARD" });
      const r = data as Record<string, unknown>;
      const tenant = (r.tenant ?? r) as Record<string, unknown>;
      const provisioning = r.provisioning as Record<string, unknown> | undefined;

      const lines = [
        `Tenant provisioning initiated.`,
        ``,
        `Tenant:`,
        `  ID     : ${tenant.id ?? id}`,
        `  Name   : ${tenant.name ?? name}`,
        `  Status : ${tenant.status ?? "ONBOARDING"}`,
      ];

      if (provisioning) {
        lines.push(``, `Provisioning Result:`);
        lines.push(`  Status   : ${provisioning.status ?? "—"}`);

        const schemas = provisioning.schemas;
        if (Array.isArray(schemas) && schemas.length > 0) {
          lines.push(``, `  Schemas created:`);
          for (const s of schemas as unknown[]) {
            lines.push(`    - ${s}`);
          }
        }

        const topics = provisioning.topics;
        if (Array.isArray(topics) && topics.length > 0) {
          lines.push(``, `  Kafka topics created:`);
          for (const tp of topics as unknown[]) {
            lines.push(`    - ${tp}`);
          }
        }

        const config = provisioning.config ?? provisioning.service_config;
        if (config) {
          lines.push(``, `  Service Config : ${JSON.stringify(config)}`);
        }
      }

      return { content: [{ type: "text" as const, text: lines.join("\n") }] };
    },
  );
}
