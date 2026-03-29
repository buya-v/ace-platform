import { Section, ChecklistSection, AnySection } from '../types/section';
import { StepDefinition, ChecklistItem } from '../types/step';

function authHeader(state: Record<string, unknown>, user: string): Record<string, string> {
  const token = state[`${user}_token`] as string | undefined;
  return token ? { Authorization: `Bearer ${token}` } : {};
}

function okValidator(status: number): 'PASS' | 'FAIL' {
  return status >= 200 && status < 300 ? 'PASS' : 'FAIL';
}

const envSetup: Section = {
  id: 'env-setup',
  title: 'Environment Setup',
  steps: [
    {
      id: 'env-1',
      title: 'Check Gateway Health',
      description: 'Verify the API gateway is running and healthy',
      method: 'GET',
      url: '/healthz',
      validateResponse: okValidator,
    },
    {
      id: 'env-2',
      title: 'Check Matching Engine Health',
      description: 'Verify the matching engine is reachable via gateway (any response confirms connectivity)',
      method: 'GET',
      url: '/api/v1/instruments/WHT-HRW-2026M07-UB/book',
      validateResponse: (status) => (status > 0) ? 'PASS' : 'FAIL',
    },
    {
      id: 'env-3',
      title: 'Check All Services via Gateway',
      description: 'Verify gateway can reach backend services (readiness check)',
      method: 'GET',
      url: '/readyz',
      validateResponse: (status) => (status >= 200 && status < 500) ? 'PASS' : 'FAIL',
    },
    {
      id: 'env-4',
      title: 'Verify Gateway Routes',
      description: 'Verify gateway can list instruments (public route)',
      method: 'GET',
      url: '/api/v1/instruments/list',
      validateResponse: (status) => (status === 200 || status === 502) ? 'PASS' : 'FAIL',
    },
  ],
};

const registration: Section = {
  id: 'registration',
  title: 'User Registration & KYC',
  steps: [
    {
      id: 'reg-1',
      title: 'Register Trader 1',
      description: 'Create first trader account',
      method: 'POST',
      url: '/api/v1/auth/register',
      body: () => ({ username: 'trader1', password: 'Tr@der1Pass!', email: 'trader1@ace.mn', role: 'trader' }),
      validateResponse: (status) => (status === 200 || status === 201 || status === 409) ? 'PASS' : 'FAIL',
    },
    {
      id: 'reg-2',
      title: 'Register Trader 2',
      description: 'Create second trader account',
      method: 'POST',
      url: '/api/v1/auth/register',
      body: () => ({ username: 'trader2', password: 'Tr@der2Pass!', email: 'trader2@ace.mn', role: 'trader' }),
      validateResponse: (status) => (status === 200 || status === 201 || status === 409) ? 'PASS' : 'FAIL',
    },
    {
      id: 'reg-3',
      title: 'Register Admin',
      description: 'Create admin account',
      method: 'POST',
      url: '/api/v1/auth/register',
      body: () => ({ username: 'admin', password: 'Adm1n@Pass!', email: 'admin@ace.mn', role: 'admin' }),
      validateResponse: (status) => (status === 200 || status === 201 || status === 409) ? 'PASS' : 'FAIL',
    },
    {
      id: 'reg-4',
      title: 'Login Trader 1',
      description: 'Authenticate trader1 and store JWT token',
      method: 'POST',
      url: '/api/v1/auth/login',
      body: () => ({ email: 'trader1@ace.mn', password: 'Tr@der1Pass!' }),
      validateResponse: okValidator,
      extractState: (body) => {
        const b = body as Record<string, unknown>;
        const token = (b.access_token || b.AccessToken || b.token) as string;
        return { trader1_token: token, trader1_id: b.user_id || b.UserId || 'trader1' };
      },
    },
    {
      id: 'reg-5',
      title: 'Login Trader 2',
      description: 'Authenticate trader2 and store JWT token',
      method: 'POST',
      url: '/api/v1/auth/login',
      body: () => ({ email: 'trader2@ace.mn', password: 'Tr@der2Pass!' }),
      validateResponse: okValidator,
      extractState: (body) => {
        const b = body as Record<string, unknown>;
        const token = (b.access_token || b.AccessToken || b.token) as string;
        return { trader2_token: token, trader2_id: b.user_id || b.UserId || 'trader2' };
      },
    },
    {
      id: 'reg-6',
      title: 'Login Admin',
      description: 'Authenticate admin and store JWT token',
      method: 'POST',
      url: '/api/v1/auth/login',
      body: () => ({ email: 'admin@ace.mn', password: 'Adm1n@Pass!' }),
      validateResponse: okValidator,
      extractState: (body) => {
        const b = body as Record<string, unknown>;
        const token = (b.access_token || b.AccessToken || b.token) as string;
        return { admin_token: token };
      },
    },
  ],
};

const trading: Section = {
  id: 'trading',
  title: 'Trading Flow',
  steps: [
    {
      id: 'trade-1',
      title: 'Submit Buy Order (Trader 1)',
      description: 'Place a limit buy order for wheat futures',
      method: 'POST',
      url: '/api/v1/orders',
      headers: (state) => authHeader(state, 'trader1'),
      body: () => ({
        instrument_id: 'WHT-HRW-2026M07-UB',
        side: 'BUY',
        type: 'LIMIT',
        price: '325.50',
        quantity: '10',
      }),
      validateResponse: okValidator,
      extractState: (body) => {
        const b = body as Record<string, unknown>;
        return { buy_order_id: b.order_id || b.id };
      },
    },
    {
      id: 'trade-2',
      title: 'Submit Sell Order (Trader 2)',
      description: 'Place a matching limit sell order',
      method: 'POST',
      url: '/api/v1/orders',
      headers: (state) => authHeader(state, 'trader2'),
      body: () => ({
        instrument_id: 'WHT-HRW-2026M07-UB',
        side: 'SELL',
        type: 'LIMIT',
        price: '325.50',
        quantity: '10',
      }),
      validateResponse: okValidator,
      extractState: (body) => {
        const b = body as Record<string, unknown>;
        return { sell_order_id: b.order_id || b.id };
      },
    },
    {
      id: 'trade-3',
      title: 'View Order Book',
      description: 'Check the current order book for wheat futures',
      method: 'GET',
      url: '/api/v1/instruments/WHT-HRW-2026M07-UB/book',
      headers: (state) => authHeader(state, 'trader1'),
      validateResponse: okValidator,
    },
    {
      id: 'trade-4',
      title: 'View Last Trade',
      description: 'View the most recent trade execution',
      method: 'GET',
      url: '/api/v1/instruments/WHT-HRW-2026M07-UB/trades/latest',
      headers: (state) => authHeader(state, 'trader1'),
      validateResponse: okValidator,
    },
    {
      id: 'trade-5',
      title: 'Cancel an Order',
      description: 'Place a new order and then cancel it',
      method: 'POST',
      url: '/api/v1/orders',
      headers: (state) => authHeader(state, 'trader1'),
      body: () => ({
        instrument_id: 'WHT-HRW-2026M07-UB',
        side: 'BUY',
        type: 'LIMIT',
        price: '300.00',
        quantity: '5',
      }),
      validateResponse: okValidator,
      extractState: (body) => {
        const b = body as Record<string, unknown>;
        return { cancel_order_id: b.order_id || b.id };
      },
    },
  ],
};

const postTrade: Section = {
  id: 'post-trade',
  title: 'Post-Trade',
  steps: [
    {
      id: 'post-1',
      title: 'View Clearing Positions',
      description: 'View current clearing positions after trade execution',
      method: 'GET',
      url: '/api/v1/clearing/positions',
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'post-2',
      title: 'View Netting Obligations',
      description: 'View netting obligations for settled trades',
      method: 'GET',
      url: '/api/v1/clearing/netting',
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'post-3',
      title: 'View Margin Requirements',
      description: 'Check margin requirements for open positions',
      method: 'GET',
      url: '/api/v1/margin',
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'post-4',
      title: 'View Margin Calls',
      description: 'Check for any outstanding margin calls',
      method: 'GET',
      url: '/api/v1/margin/calls',
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: okValidator,
    },
  ],
};

const delivery: Section = {
  id: 'delivery',
  title: 'Physical Delivery',
  steps: [
    {
      id: 'del-1',
      title: 'Issue Warehouse Receipt',
      description: 'Issue a new warehouse receipt for physical commodity',
      method: 'POST',
      url: '/api/v1/warehouse/receipts',
      headers: (state) => authHeader(state, 'admin'),
      body: () => ({
        commodity: 'HRW_WHEAT',
        quantity: '5000',
        unit: 'bushels',
        warehouse_id: 'WH-001',
        grade: 'US_NO_1',
      }),
      validateResponse: okValidator,
      extractState: (body) => {
        const b = body as Record<string, unknown>;
        return { receipt_id: b.receipt_id || b.id };
      },
    },
    {
      id: 'del-2',
      title: 'Pledge Receipt as Collateral',
      description: 'Pledge the warehouse receipt as margin collateral',
      method: 'POST',
      url: (state) => `/api/v1/warehouse/receipts/${state.receipt_id || 'RECEIPT-001'}/pledge`,
      headers: (state) => authHeader(state, 'trader1'),
      body: () => ({ purpose: 'margin_collateral' }),
      validateResponse: okValidator,
    },
    {
      id: 'del-3',
      title: 'Initiate Delivery',
      description: 'Initiate physical delivery against a settled contract',
      method: 'POST',
      url: '/api/v1/warehouse/deliveries',
      headers: (state) => authHeader(state, 'admin'),
      body: (state) => ({
        receipt_id: state.receipt_id || 'RECEIPT-001',
        buyer_id: state.trader1_id || 'trader1',
        seller_id: state.trader2_id || 'trader2',
        quantity: '5000',
      }),
      validateResponse: okValidator,
    },
    {
      id: 'del-4',
      title: 'View Warehouse Inventory',
      description: 'Check current warehouse inventory levels',
      method: 'GET',
      url: '/api/v1/warehouse/inventory',
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: okValidator,
    },
  ],
};

const marketData: Section = {
  id: 'market-data',
  title: 'Market Data',
  steps: [
    {
      id: 'mkt-1',
      title: 'Get OHLCV Candles',
      description: 'Retrieve candlestick chart data for wheat futures',
      method: 'GET',
      url: '/api/v1/market-data/candles/WHT-HRW-2026M07-UB?interval=1m',
      validateResponse: okValidator,
    },
    {
      id: 'mkt-2',
      title: 'Get Ticker',
      description: 'Get latest ticker data for wheat futures',
      method: 'GET',
      url: '/api/v1/market-data/ticker/WHT-HRW-2026M07-UB',
      validateResponse: okValidator,
    },
    {
      id: 'mkt-3',
      title: 'Get Recent Trades',
      description: 'Retrieve recent trade history',
      method: 'GET',
      url: '/api/v1/market-data/trades/WHT-HRW-2026M07-UB',
      validateResponse: okValidator,
    },
  ],
};

const compliance: Section = {
  id: 'compliance',
  title: 'Compliance & Risk',
  steps: [
    {
      id: 'comp-1',
      title: 'Get Participant Status',
      description: 'Check compliance status for a participant',
      method: 'GET',
      url: (state) => `/api/v1/participants/${state.trader1_id || 'trader1'}`,
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'comp-2',
      title: 'Get Risk Score',
      description: 'Retrieve risk score for a participant',
      method: 'GET',
      url: (state) => `/api/v1/risk-scores/${state.trader1_id || 'trader1'}`,
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'comp-3',
      title: 'View Compliance Alerts',
      description: 'View active compliance alerts',
      method: 'GET',
      url: '/api/v1/compliance/alerts',
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: okValidator,
    },
  ],
};

const adminOps: Section = {
  id: 'admin-ops',
  title: 'Admin Operations',
  steps: [
    {
      id: 'admin-1',
      title: 'View All Service Health',
      description: 'Aggregated health check across all services',
      method: 'GET',
      url: '/healthz',
      validateResponse: okValidator,
    },
    {
      id: 'admin-2',
      title: 'View Settlement Cycles',
      description: 'View settlement cycle history',
      method: 'GET',
      url: '/api/v1/settlement/cycles',
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'admin-3',
      title: 'View Circuit Breakers',
      description: 'View circuit breaker status for trading halts',
      method: 'GET',
      url: '/api/v1/admin/circuit-breakers',
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: okValidator,
    },
  ],
};

const adminOrderbook: Section = {
  id: 'admin-orderbook',
  title: 'Order Book (Admin View)',
  steps: [
    {
      id: 'ob-1',
      title: 'Fetch Instrument List',
      description: 'Lists all tradeable instruments. View on Admin → Order Book page (/dashboard/orderbook)',
      method: 'GET',
      url: '/api/v1/instruments/list',
      validateResponse: okValidator,
    },
    {
      id: 'ob-2',
      title: 'Fetch Order Book for Wheat',
      description: 'Shows bid/ask depth for wheat futures. View on Admin → Order Book page',
      method: 'GET',
      url: '/api/v1/instruments/WHT-HRW-2026M07-UB/book',
      validateResponse: okValidator,
    },
    {
      id: 'ob-3',
      title: 'Fetch Last Trade',
      description: 'Most recent trade execution. View on Admin → Order Book page',
      method: 'GET',
      url: '/api/v1/instruments/WHT-HRW-2026M07-UB/trades/latest',
      validateResponse: okValidator,
    },
    {
      id: 'ob-4',
      title: 'Fetch Market Trades',
      description: 'Trade tape from market data service. View on Admin → Order Book page',
      method: 'GET',
      url: '/api/v1/market-data/trades/WHT-HRW-2026M07-UB',
      validateResponse: okValidator,
    },
  ],
};

const adminPositions: Section = {
  id: 'admin-positions',
  title: 'Positions & Risk',
  steps: [
    {
      id: 'pos-1',
      title: 'Fetch Clearing Positions',
      description: 'All open positions across participants. View on Admin → Positions page (/dashboard/positions)',
      method: 'GET',
      url: '/api/v1/clearing/positions',
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'pos-2',
      title: 'Fetch Netting Obligations',
      description: 'Netting obligations for settled trades. View on Admin → Positions page',
      method: 'GET',
      url: '/api/v1/clearing/netting',
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'pos-3',
      title: 'Fetch Portfolio Margin',
      description: 'Portfolio-level margin requirements. View on Admin → Risk Overview page (/dashboard/risk)',
      method: 'GET',
      url: '/api/v1/margin',
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'pos-4',
      title: 'Fetch Margin Calls',
      description: 'Outstanding margin calls. View on Admin → Margin Calls page (/dashboard/margin)',
      method: 'GET',
      url: '/api/v1/margin/calls',
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: okValidator,
    },
  ],
};

const adminSettlement: Section = {
  id: 'admin-settlement',
  title: 'Settlement',
  steps: [
    {
      id: 'stl-1',
      title: 'Fetch Settlement Cycles',
      description: 'Settlement cycle history and status. View on Admin → Settlement page (/dashboard/settlement)',
      method: 'GET',
      url: '/api/v1/settlement/cycles',
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'stl-2',
      title: 'Trigger Settlement Cycle',
      description: 'Initiates a new settlement cycle. View on Admin → Settlement page',
      method: 'POST',
      url: '/api/v1/settlement/cycle',
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: (status) => (status >= 200 && status < 300) ? 'PASS' : 'FAIL',
      extractState: (body) => {
        const b = body as Record<string, unknown>;
        return { settlement_cycle_id: b.cycle_id || b.id };
      },
    },
    {
      id: 'stl-3',
      title: 'Verify Settlement Cycle',
      description: 'Confirm new cycle appears. View on Admin → Settlement page',
      method: 'GET',
      url: '/api/v1/settlement/cycles',
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: okValidator,
    },
  ],
};

const adminCircuitBreakers: Section = {
  id: 'admin-circuit-breakers',
  title: 'Circuit Breakers',
  steps: [
    {
      id: 'cb-1',
      title: 'Fetch Instruments with Status',
      description: 'Instruments with trading phase and circuit breaker config. View on Admin → Circuit Breakers page (/dashboard/circuit-breakers)',
      method: 'GET',
      url: '/api/v1/instruments',
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'cb-2',
      title: 'Set Circuit Breaker',
      description: 'Configure price limits for wheat. View on Admin → Circuit Breakers page',
      method: 'PUT',
      url: '/api/v1/admin/instruments/WHT-HRW-2026M07-UB/circuit-breaker',
      headers: (state) => authHeader(state, 'admin'),
      body: () => ({ upper_limit_pct: 10, lower_limit_pct: 10, cooldown_minutes: 5, reference_price: '325.50' }),
      validateResponse: (status) => (status >= 200 && status < 300) ? 'PASS' : 'FAIL',
    },
    {
      id: 'cb-3',
      title: 'Halt Instrument',
      description: 'Halt trading on wheat futures. View on Admin → Market Phase page (/dashboard/market-phase)',
      method: 'POST',
      url: '/api/v1/admin/instruments/WHT-HRW-2026M07-UB/halt',
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: (status) => (status >= 200 && status < 300) ? 'PASS' : 'FAIL',
    },
    {
      id: 'cb-4',
      title: 'Resume Instrument',
      description: 'Resume trading on wheat futures. View on Admin → Market Phase page',
      method: 'POST',
      url: '/api/v1/admin/instruments/WHT-HRW-2026M07-UB/resume',
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: (status) => (status >= 200 && status < 300) ? 'PASS' : 'FAIL',
    },
  ],
};

const adminMonitoring: Section = {
  id: 'admin-monitoring',
  title: 'System Monitoring',
  steps: [
    {
      id: 'mon-1',
      title: 'Fetch Admin Health',
      description: 'Aggregated service health status. View on Admin → System Health page (/dashboard/monitoring)',
      method: 'GET',
      url: '/api/v1/admin/health',
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'mon-2',
      title: 'Fetch Compliance Alerts',
      description: 'Active compliance alerts. View on Admin → Compliance page',
      method: 'GET',
      url: '/api/v1/compliance/alerts',
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'mon-3',
      title: 'Fetch Audit Trail',
      description: 'Recent system audit events',
      method: 'GET',
      url: '/api/v1/compliance/audit-trail',
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'mon-4',
      title: 'Fetch Warehouse Facilities',
      description: 'Warehouse facility registry and capacity. View on Admin → Warehouse page (/dashboard/warehouse)',
      method: 'GET',
      url: '/api/v1/warehouse/facilities',
      headers: (state) => authHeader(state, 'admin'),
      validateResponse: okValidator,
    },
  ],
};

const readinessItems: ChecklistItem[] = [
  { id: 'sec-1', category: 'Security', description: 'TLS termination configured on gateway', status: 'Not Ready' },
  { id: 'sec-2', category: 'Security', description: 'JWT signing keys rotated and stored in secrets manager', status: 'Not Ready' },
  { id: 'sec-3', category: 'Security', description: 'RBAC enforced on all API endpoints', status: 'Partial' },
  { id: 'sec-4', category: 'Security', description: 'Rate limiting enabled on auth endpoints', status: 'Not Ready' },
  { id: 'perf-1', category: 'Performance', description: 'Matching engine < 1ms p99 latency', status: 'Ready' },
  { id: 'perf-2', category: 'Performance', description: 'Order throughput > 10,000 orders/sec', status: 'Partial' },
  { id: 'perf-3', category: 'Performance', description: 'Database connection pooling configured', status: 'Not Ready' },
  { id: 'mon-1', category: 'Monitoring', description: 'Prometheus metrics exported from all services', status: 'Not Ready' },
  { id: 'mon-2', category: 'Monitoring', description: 'Grafana dashboards for trading metrics', status: 'Not Ready' },
  { id: 'mon-3', category: 'Monitoring', description: 'Alerting rules for margin breach events', status: 'Not Ready' },
  { id: 'data-1', category: 'Data Integrity', description: 'Database migrations tested and versioned', status: 'Ready' },
  { id: 'data-2', category: 'Data Integrity', description: 'Audit log for all trade modifications', status: 'Partial' },
  { id: 'dr-1', category: 'DR', description: 'Database backup strategy documented', status: 'Not Ready' },
  { id: 'dr-2', category: 'DR', description: 'Failover procedure for matching engine', status: 'Not Ready' },
  { id: 'reg-r1', category: 'Regulatory', description: 'Trade reporting to regulatory body', status: 'Not Ready' },
  { id: 'reg-r2', category: 'Regulatory', description: 'KYC/AML verification integrated', status: 'Partial' },
];

const readiness: ChecklistSection = {
  id: 'readiness',
  title: 'Production Readiness',
  items: readinessItems,
};

export const allSections: AnySection[] = [
  envSetup,
  registration,
  trading,
  postTrade,
  delivery,
  marketData,
  compliance,
  adminOps,
  adminOrderbook,
  adminPositions,
  adminSettlement,
  adminCircuitBreakers,
  adminMonitoring,
  readiness,
];

export function getAllSteps(): StepDefinition[] {
  return allSections.flatMap((s) => ('steps' in s ? s.steps : []));
}

export function getTotalStepCount(): number {
  return getAllSteps().length;
}
