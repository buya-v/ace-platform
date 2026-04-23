import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { GatewayClient } from "../gateway-client.js";

export function registerSecuritiesTools(server: McpServer, client: GatewayClient): void {
  // ── 1. List instruments ──────────────────────────────────────────────────
  server.tool(
    "list_securities_instruments",
    "List all securities instruments with optional filters by asset class and trading status",
    {
      asset_class: z
        .enum(["EQUITY", "BOND", "ETF"])
        .optional()
        .describe("Filter by asset class (EQUITY, BOND, or ETF)"),
      trading_status: z
        .string()
        .optional()
        .describe("Filter by trading status (e.g. ACTIVE, HALTED)"),
    },
    async ({ asset_class, trading_status }) => {
      const params = new URLSearchParams();
      if (asset_class) params.set("asset_class", asset_class);
      if (trading_status) params.set("trading_status", trading_status);
      const qs = params.toString();
      const path = `/api/v1/securities/instruments${qs ? `?${qs}` : ""}`;
      const data = await client.request("GET", path);

      const items = Array.isArray(data) ? data : (data as Record<string, unknown[]>).instruments ?? [];
      if (items.length === 0) {
        return { content: [{ type: "text" as const, text: "No instruments found." }] };
      }

      const header =
        "TICKER       NAME                 ASSET CLASS  STATUS      LOT   TICK   CCY";
      const divider = "─".repeat(header.length);
      const lines = (items as Record<string, unknown>[]).map((i) => {
        const ticker = String(i.ticker ?? "").padEnd(12);
        const name = String(i.name ?? "").padEnd(20).slice(0, 20);
        const assetClass = String(i.asset_class ?? "").padEnd(12);
        const status = String(i.trading_status ?? i.status ?? "").padEnd(11);
        const lotSize = String(i.lot_size ?? "").padEnd(5);
        const tickSize = String(i.tick_size ?? "").padEnd(6);
        const currency = String(i.currency ?? "MNT");
        return `${ticker} ${name} ${assetClass} ${status} ${lotSize} ${tickSize} ${currency}`;
      });

      const text = [header, divider, ...lines].join("\n");
      return { content: [{ type: "text" as const, text }] };
    },
  );

  // ── 2. Get a single instrument ───────────────────────────────────────────
  server.tool(
    "get_securities_instrument",
    "Get details of a specific securities instrument by ID",
    {
      instrument_id: z.string().describe("Instrument ID (e.g., MNGL-001)"),
    },
    async ({ instrument_id }) => {
      const data = await client.request(
        "GET",
        `/api/v1/securities/instruments/${instrument_id}`,
      );
      const i = data as Record<string, unknown>;
      const lines = [
        `Instrument ID : ${i.id ?? instrument_id}`,
        `Ticker        : ${i.ticker ?? "—"}`,
        `Name          : ${i.name ?? "—"}`,
        `Asset Class   : ${i.asset_class ?? "—"}`,
        `Status        : ${i.trading_status ?? i.status ?? "—"}`,
        `Lot Size      : ${i.lot_size ?? "—"}`,
        `Tick Size     : ${i.tick_size ?? "—"}`,
        `Currency      : ${i.currency ?? "MNT"}`,
        `Created At    : ${i.created_at ?? "—"}`,
        `Updated At    : ${i.updated_at ?? "—"}`,
      ];
      return { content: [{ type: "text" as const, text: lines.join("\n") }] };
    },
  );

  // ── 3. Create an instrument ──────────────────────────────────────────────
  server.tool(
    "create_securities_instrument",
    "Create a new securities instrument (equity, bond, or ETF)",
    {
      ticker: z.string().describe("Ticker symbol (e.g., MNT-001)"),
      name: z.string().describe("Full name of the instrument"),
      asset_class: z
        .enum(["EQUITY", "BOND", "ETF"])
        .describe("Asset class: EQUITY, BOND, or ETF"),
      lot_size: z.number().positive().describe("Minimum tradable lot size"),
      tick_size: z
        .number()
        .positive()
        .describe("Minimum price increment (tick size)"),
      currency: z
        .string()
        .default("MNT")
        .describe("Settlement currency (default: MNT)"),
    },
    async ({ ticker, name, asset_class, lot_size, tick_size, currency }) => {
      const body = { ticker, name, asset_class, lot_size, tick_size, currency };
      const data = await client.request(
        "POST",
        "/api/v1/securities/instruments",
        body,
      );
      const i = data as Record<string, unknown>;
      const lines = [
        `Instrument created successfully.`,
        ``,
        `ID          : ${i.id ?? "—"}`,
        `Ticker      : ${i.ticker ?? ticker}`,
        `Name        : ${i.name ?? name}`,
        `Asset Class : ${i.asset_class ?? asset_class}`,
        `Lot Size    : ${i.lot_size ?? lot_size}`,
        `Tick Size   : ${i.tick_size ?? tick_size}`,
        `Currency    : ${i.currency ?? currency}`,
        `Status      : ${i.trading_status ?? i.status ?? "ACTIVE"}`,
      ];
      return { content: [{ type: "text" as const, text: lines.join("\n") }] };
    },
  );

  // ── 4. Submit an order ───────────────────────────────────────────────────
  server.tool(
    "submit_securities_order",
    "Submit a buy or sell order for a securities instrument",
    {
      instrument_id: z
        .string()
        .describe("Instrument ID to trade (e.g., MNGL-001)"),
      side: z.enum(["BUY", "SELL"]).describe("Order side: BUY or SELL"),
      order_type: z
        .enum(["LIMIT", "MARKET"])
        .describe("Order type: LIMIT or MARKET"),
      quantity: z.number().positive().describe("Number of lots to trade"),
      price: z
        .number()
        .positive()
        .optional()
        .describe("Limit price (required for LIMIT orders, omit for MARKET)"),
    },
    async ({ instrument_id, side, order_type, quantity, price }) => {
      const body: Record<string, unknown> = {
        instrument_id,
        side,
        order_type,
        quantity,
      };
      if (price !== undefined) body.price = price;
      const data = await client.request(
        "POST",
        "/api/v1/securities/orders",
        body,
      );
      const o = data as Record<string, unknown>;
      const lines = [
        `Order submitted.`,
        ``,
        `Order ID    : ${o.id ?? "—"}`,
        `Instrument  : ${o.instrument_id ?? instrument_id}`,
        `Side        : ${o.side ?? side}`,
        `Type        : ${o.order_type ?? order_type}`,
        `Quantity    : ${o.quantity ?? quantity}`,
        `Price       : ${o.price ?? price ?? "MARKET"}`,
        `Status      : ${o.status ?? "PENDING"}`,
        `Created At  : ${o.created_at ?? "—"}`,
      ];
      return { content: [{ type: "text" as const, text: lines.join("\n") }] };
    },
  );

  // ── 5. List positions ────────────────────────────────────────────────────
  server.tool(
    "list_securities_positions",
    "List securities positions, optionally filtered by participant",
    {
      participant_id: z
        .string()
        .optional()
        .describe("Filter by participant ID"),
    },
    async ({ participant_id }) => {
      const params = new URLSearchParams();
      if (participant_id) params.set("participant_id", participant_id);
      const qs = params.toString();
      const path = `/api/v1/securities/positions${qs ? `?${qs}` : ""}`;
      const data = await client.request("GET", path);

      const items = Array.isArray(data)
        ? data
        : (data as Record<string, unknown[]>).positions ?? [];
      if (items.length === 0) {
        return { content: [{ type: "text" as const, text: "No positions found." }] };
      }

      const header =
        "INSTRUMENT        QTY         AVG COST       P&L            PARTICIPANT";
      const divider = "─".repeat(header.length);
      const lines = (items as Record<string, unknown>[]).map((p) => {
        const instrument = String(p.instrument_id ?? p.ticker ?? "").padEnd(17);
        const qty = String(p.quantity ?? p.qty ?? "").padStart(10);
        const avgCost = String(p.avg_cost ?? p.average_cost ?? "—").padStart(14);
        const pnl = String(p.unrealized_pnl ?? p.pnl ?? "—").padStart(14);
        const participant = String(p.participant_id ?? "—");
        return `${instrument} ${qty} ${avgCost} ${pnl} ${participant}`;
      });

      const text = [header, divider, ...lines].join("\n");
      return { content: [{ type: "text" as const, text }] };
    },
  );

  // ── 6. List orders ───────────────────────────────────────────────────────
  server.tool(
    "list_securities_orders",
    "List securities orders with optional filters by instrument and status",
    {
      instrument_id: z
        .string()
        .optional()
        .describe("Filter by instrument ID"),
      status: z
        .string()
        .optional()
        .describe(
          "Filter by order status (e.g. PENDING, FILLED, CANCELLED, PARTIAL)",
        ),
    },
    async ({ instrument_id, status }) => {
      const params = new URLSearchParams();
      if (instrument_id) params.set("instrument_id", instrument_id);
      if (status) params.set("status", status);
      const qs = params.toString();
      const path = `/api/v1/securities/orders${qs ? `?${qs}` : ""}`;
      const data = await client.request("GET", path);

      const items = Array.isArray(data)
        ? data
        : (data as Record<string, unknown[]>).orders ?? [];
      if (items.length === 0) {
        return { content: [{ type: "text" as const, text: "No orders found." }] };
      }

      const header =
        "ORDER ID              INSTRUMENT   SIDE   TYPE     QTY      PRICE      STATUS";
      const divider = "─".repeat(header.length);
      const lines = (items as Record<string, unknown>[]).map((o) => {
        const id = String(o.id ?? "").padEnd(21);
        const instr = String(o.instrument_id ?? "").padEnd(12);
        const side = String(o.side ?? "").padEnd(6);
        const type = String(o.order_type ?? o.type ?? "").padEnd(8);
        const qty = String(o.quantity ?? "").padStart(8);
        const price = String(o.price ?? "MARKET").padStart(10);
        const orderStatus = String(o.status ?? "");
        return `${id} ${instr} ${side} ${type} ${qty} ${price} ${orderStatus}`;
      });

      const text = [header, divider, ...lines].join("\n");
      return { content: [{ type: "text" as const, text }] };
    },
  );

  // ── 7. Cancel an order ───────────────────────────────────────────────────
  server.tool(
    "cancel_securities_order",
    "Cancel an open securities order by its ID",
    {
      order_id: z.string().describe("Order ID to cancel"),
    },
    async ({ order_id }) => {
      const data = await client.request(
        "DELETE",
        `/api/v1/securities/orders/${order_id}`,
      );
      const result = data as Record<string, unknown>;
      const lines = [
        `Order cancelled.`,
        ``,
        `Order ID : ${result.id ?? order_id}`,
        `Status   : ${result.status ?? "CANCELLED"}`,
        `Message  : ${result.message ?? "Order successfully cancelled"}`,
      ];
      return { content: [{ type: "text" as const, text: lines.join("\n") }] };
    },
  );

  // ── 8. List settlement obligations ──────────────────────────────────────
  server.tool(
    "list_settlement_obligations",
    "List settlement obligations with optional date and status filters",
    {
      date: z.string().optional().describe("Settlement date (YYYY-MM-DD)"),
      status: z.string().optional().describe("Status filter (PENDING/AFFIRMED/NETTED/SETTLED/FAILED)"),
    },
    async ({ date, status }) => {
      const params = new URLSearchParams();
      if (date) params.set("date", date);
      if (status) params.set("status", status);
      const qs = params.toString();
      const result = await client.request("GET", `/api/v1/securities/settlements${qs ? "?" + qs : ""}`);
      const items = Array.isArray(result) ? result : (result as any).data ?? [];
      if (items.length === 0) return { content: [{ type: "text" as const, text: "No settlement obligations found." }] };
      const header = "Trade ID".padEnd(38) + "Instrument".padEnd(14) + "Qty".padStart(8) + "  " + "Amount".padStart(14) + "  " + "Status".padEnd(10) + "Date";
      const divider = "─".repeat(header.length);
      const rows = items.map((o: any) =>
        (o.trade_id ?? "—").toString().padEnd(38) +
        (o.instrument_id ?? "—").toString().padEnd(14) +
        (o.quantity ?? 0).toString().padStart(8) + "  " +
        (o.net_amount ?? 0).toFixed(2).padStart(14) + "  " +
        (o.status ?? "—").padEnd(10) +
        (o.settlement_date ?? "—")
      );
      return { content: [{ type: "text" as const, text: [header, divider, ...rows].join("\n") }] };
    },
  );

  // ── 9. Trigger settlement cycle ─────────────────────────────────────────
  server.tool(
    "trigger_settlement_cycle",
    "Trigger a settlement cycle for a specific date — processes all pending obligations",
    {
      date: z.string().describe("Settlement date to process (YYYY-MM-DD)"),
    },
    async ({ date }) => {
      const result = await client.request("POST", "/api/v1/securities/settlements/cycle", { date });
      const r = result as any;
      const lines = [
        `Settlement Cycle — ${date}`,
        `─────────────────────────────`,
        `Processed : ${r.processed ?? 0}`,
        `Affirmed  : ${r.affirmed ?? 0}`,
        `Netted    : ${r.netted ?? 0}`,
        `Settled   : ${r.settled ?? 0}`,
        `Failed    : ${r.failed ?? 0}`,
      ];
      return { content: [{ type: "text" as const, text: lines.join("\n") }] };
    },
  );
}
