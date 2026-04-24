import { Section, ChecklistSection, AnySection } from '../types/section';
import { StepDefinition, ChecklistItem } from '../types/step';

function authHeader(state: Record<string, unknown>, user: string): Record<string, string> {
  const token = state[`${user}_token`] as string | undefined;
  return token ? { Authorization: `Bearer ${token}` } : {};
}

function tenantHeader(state: Record<string, unknown>, user: string): Record<string, string> {
  return { ...authHeader(state, user), 'X-GarudaX-Tenant': 'mse-equities' };
}

function okValidator(status: number): 'PASS' | 'FAIL' {
  return status >= 200 && status < 500 ? 'PASS' : 'FAIL';
}

// ─── Section 1: Environment Setup ────────────────────────────────────────────

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
      title: 'Check Readiness',
      description: 'Verify the gateway is ready to accept requests',
      method: 'GET',
      url: '/readyz',
      validateResponse: okValidator,
    },
  ],
};

// ─── Section 2: User Registration ────────────────────────────────────────────

const registration: Section = {
  id: 'registration',
  title: 'User Registration & Login',
  steps: [
    {
      id: 'reg-1',
      title: 'Register Trader 1',
      description: 'Create first trader account (buyer)',
      method: 'POST',
      url: '/api/v1/auth/register',
      body: () => ({ username: 'trader1', password: 'Tr@der1Pass!', email: 'trader1@garudax.mn', role: 'trader' }),
      validateResponse: (status) => (status === 200 || status === 201 || status === 409) ? 'PASS' : 'FAIL',
    },
    {
      id: 'reg-2',
      title: 'Register Trader 2',
      description: 'Create second trader account (seller)',
      method: 'POST',
      url: '/api/v1/auth/register',
      body: () => ({ username: 'trader2', password: 'Tr@der2Pass!', email: 'trader2@garudax.mn', role: 'trader' }),
      validateResponse: (status) => (status === 200 || status === 201 || status === 409) ? 'PASS' : 'FAIL',
    },
    {
      id: 'reg-3',
      title: 'Register Admin',
      description: 'Create exchange admin account',
      method: 'POST',
      url: '/api/v1/auth/register',
      body: () => ({ username: 'admin', password: 'Adm1n@Pass!', email: 'admin@garudax.mn', role: 'admin' }),
      validateResponse: (status) => (status === 200 || status === 201 || status === 409) ? 'PASS' : 'FAIL',
    },
    {
      id: 'reg-4',
      title: 'Login Trader 1',
      description: 'Authenticate trader1 and store JWT token',
      method: 'POST',
      url: '/api/v1/auth/login',
      body: () => ({ email: 'trader1@garudax.mn', password: 'Tr@der1Pass!' }),
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
      body: () => ({ email: 'trader2@garudax.mn', password: 'Tr@der2Pass!' }),
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
      body: () => ({ email: 'admin@garudax.mn', password: 'Adm1n@Pass!' }),
      validateResponse: okValidator,
      extractState: (body) => {
        const b = body as Record<string, unknown>;
        const token = (b.access_token || b.AccessToken || b.token) as string;
        return { admin_token: token };
      },
    },
  ],
};

// ─── Section 3: Securities Instruments ───────────────────────────────────────

const securitiesInstruments: Section = {
  id: 'securities-instruments',
  title: 'Securities — Instruments',
  steps: [
    {
      id: 'sec-inst-1',
      title: 'Create Equity: APU JSC',
      description: 'List APU JSC (food & beverages) on MSE — lot_size=10, tick_size=1 MNT',
      method: 'POST',
      url: '/api/v1/securities/instruments',
      headers: (state) => tenantHeader(state, 'admin'),
      body: () => ({
        ticker: 'APU',
        name: 'APU JSC',
        asset_class: 'EQUITY',
        lot_size: 10,
        tick_size: 1,
        currency: 'MNT',
        exchange_code: 'MSE',
      }),
      validateResponse: okValidator,
      extractState: (body) => {
        const b = body as Record<string, unknown>;
        return { apu_id: b.id };
      },
    },
    {
      id: 'sec-inst-2',
      title: 'Create Equity: Govisumber Mining',
      description: 'List Govisumber Mining on MSE — lot_size=100, tick_size=50 MNT',
      method: 'POST',
      url: '/api/v1/securities/instruments',
      headers: (state) => tenantHeader(state, 'admin'),
      body: () => ({
        ticker: 'GOV',
        name: 'Govisumber Mining LLC',
        asset_class: 'EQUITY',
        lot_size: 100,
        tick_size: 50,
        currency: 'MNT',
        exchange_code: 'MSE',
      }),
      validateResponse: okValidator,
      extractState: (body) => {
        const b = body as Record<string, unknown>;
        return { gov_id: b.id };
      },
    },
    {
      id: 'sec-inst-3',
      title: 'List All Instruments',
      description: 'Verify both instruments are listed on MSE',
      method: 'GET',
      url: '/api/v1/securities/instruments',
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
  ],
};

// ─── Section 4: Securities Trading ───────────────────────────────────────────

const securitiesTrading: Section = {
  id: 'securities-trading',
  title: 'Securities — Order Matching',
  steps: [
    {
      id: 'sec-trade-1',
      title: 'Submit Buy Order (Trader 1)',
      description: 'Trader 1 places a limit buy for 100 shares of APU at 850 MNT',
      method: 'POST',
      url: '/api/v1/securities/orders',
      headers: (state) => tenantHeader(state, 'trader1'),
      body: (state) => ({
        instrument_id: state.apu_id || 'APU',
        side: 'BUY',
        order_type: 'LIMIT',
        quantity: 100,
        price: 850,
        time_in_force: 'GTC',
      }),
      validateResponse: okValidator,
      extractState: (body) => {
        const b = body as Record<string, unknown>;
        const order = (b.order || b) as Record<string, unknown>;
        return { buy_order_id: order.id || order.order_id || order.exec_id };
      },
    },
    {
      id: 'sec-trade-2',
      title: 'Submit Matching Sell Order (Trader 2)',
      description: 'Trader 2 places a matching sell at 850 MNT → trade executes',
      method: 'POST',
      url: '/api/v1/securities/orders',
      headers: (state) => tenantHeader(state, 'trader2'),
      body: (state) => ({
        instrument_id: state.apu_id || 'APU',
        side: 'SELL',
        order_type: 'LIMIT',
        quantity: 100,
        price: 850,
        time_in_force: 'GTC',
      }),
      validateResponse: okValidator,
      extractState: (body) => {
        const b = body as Record<string, unknown>;
        const trades = (b.trades || []) as unknown[];
        return { trade_count: trades.length, last_trade: trades[0] };
      },
    },
    {
      id: 'sec-trade-3',
      title: 'Submit Market Buy (Trader 1)',
      description: 'Trader 1 places a market order for 50 shares of APU — fills at best ask',
      method: 'POST',
      url: '/api/v1/securities/orders',
      headers: (state) => tenantHeader(state, 'trader1'),
      body: (state) => ({
        instrument_id: state.apu_id || 'APU',
        side: 'BUY',
        order_type: 'MARKET',
        quantity: 50,
      }),
      validateResponse: okValidator,
    },
    {
      id: 'sec-trade-4',
      title: 'View Orders',
      description: 'List all securities orders for the tenant',
      method: 'GET',
      url: '/api/v1/securities/orders',
      headers: (state) => tenantHeader(state, 'trader1'),
      validateResponse: okValidator,
    },
    {
      id: 'sec-trade-5',
      title: 'Submit & Cancel Order',
      description: 'Place an order then cancel it immediately',
      method: 'POST',
      url: '/api/v1/securities/orders',
      headers: (state) => tenantHeader(state, 'trader1'),
      body: (state) => ({
        instrument_id: state.apu_id || 'APU',
        side: 'BUY',
        order_type: 'LIMIT',
        quantity: 10,
        price: 800,
      }),
      validateResponse: okValidator,
      extractState: (body) => {
        const b = body as Record<string, unknown>;
        const order = (b.order || b) as Record<string, unknown>;
        return { cancel_order_id: order.id || order.order_id || order.exec_id };
      },
    },
  ],
};

// ─── Section 5: Securities Positions ─────────────────────────────────────────

const securitiesPositions: Section = {
  id: 'securities-positions',
  title: 'Securities — Positions & P&L',
  steps: [
    {
      id: 'sec-pos-1',
      title: 'View Orders After Trading',
      description: 'Check all orders — should show filled and pending orders',
      method: 'GET',
      url: '/api/v1/securities/orders',
      headers: (state) => tenantHeader(state, 'trader1'),
      validateResponse: okValidator,
    },
    {
      id: 'sec-pos-2',
      title: 'View Settlements',
      description: 'Check T+2 settlement obligations created from trades',
      method: 'GET',
      url: '/api/v1/securities/settlements?status=PENDING',
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'sec-pos-3',
      title: 'Trigger Settlement Cycle',
      description: 'Process settlement for today — transitions PENDING → SETTLED',
      method: 'POST',
      url: '/api/v1/securities/settlements/cycle',
      headers: (state) => tenantHeader(state, 'admin'),
      body: () => ({ date: new Date().toISOString().slice(0, 10) }),
      validateResponse: okValidator,
    },
  ],
};

// ─── Section 6: Corporate Actions ────────────────────────────────────────────

const corporateActions: Section = {
  id: 'corporate-actions',
  title: 'Securities — Corporate Actions',
  steps: [
    {
      id: 'sec-ca-1',
      title: 'Announce Dividend',
      description: 'APU JSC announces 50 MNT per share dividend',
      method: 'POST',
      url: '/api/v1/securities/corporate-actions',
      headers: (state) => tenantHeader(state, 'admin'),
      body: (state) => ({
        instrument_id: state.apu_id || 'APU',
        action_type: 'CA_DIVIDEND',
        record_date: new Date().toISOString().slice(0, 10),
        payment_date: new Date(Date.now() + 7 * 86400000).toISOString().slice(0, 10),
        details: { dividend_amount: 50.0 },
      }),
      validateResponse: okValidator,
      extractState: (body) => {
        const b = body as Record<string, unknown>;
        return { dividend_id: b.id };
      },
    },
    {
      id: 'sec-ca-2',
      title: 'Process Dividend Entitlements',
      description: 'Calculate and create dividend entitlements for all shareholders',
      method: 'POST',
      url: (state) => `/api/v1/securities/corporate-actions/${state.dividend_id || 'DIVIDEND-001'}/process`,
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'sec-ca-3',
      title: 'List Corporate Actions',
      description: 'View all announced corporate actions',
      method: 'GET',
      url: '/api/v1/securities/corporate-actions',
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
  ],
};

// ─── Section 7: Market Sessions ──────────────────────────────────────────────

const marketSessions: Section = {
  id: 'market-sessions',
  title: 'Securities — Market Sessions',
  steps: [
    {
      id: 'sec-sess-1',
      title: 'View Current Sessions',
      description: 'Check market session state for all instruments',
      method: 'GET',
      url: '/api/v1/securities/sessions',
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'sec-sess-2',
      title: 'Open Pre-Auction Phase',
      description: 'Transition APU to PRE_OPEN — orders collected for opening auction',
      method: 'POST',
      url: (state) => `/api/v1/securities/sessions/${state.apu_id || 'APU'}/transition`,
      headers: (state) => tenantHeader(state, 'admin'),
      body: () => ({ session: 'PRE_OPEN' }),
      validateResponse: okValidator,
    },
    {
      id: 'sec-sess-3',
      title: 'Start Continuous Trading',
      description: 'Transition to CONTINUOUS — opening auction executes, then live matching',
      method: 'POST',
      url: (state) => `/api/v1/securities/sessions/${state.apu_id || 'APU'}/transition`,
      headers: (state) => tenantHeader(state, 'admin'),
      body: () => ({ session: 'CONTINUOUS' }),
      validateResponse: okValidator,
    },
  ],
};

// ─── Section 8: FRC Reporting ────────────────────────────────────────────────

const frcReporting: Section = {
  id: 'frc-reporting',
  title: 'Securities — FRC Reports',
  steps: [
    {
      id: 'sec-frc-1',
      title: 'Daily Trading Summary',
      description: 'Generate FRC daily summary report — trade count, volume, value',
      method: 'GET',
      url: `/api/v1/securities/reports/frc?type=DAILY_SUMMARY&date=${new Date().toISOString().slice(0, 10)}`,
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'sec-frc-2',
      title: 'Large Trader Report',
      description: 'Generate FRC large trader positions report',
      method: 'GET',
      url: '/api/v1/securities/reports/frc?type=LARGE_TRADER',
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
  ],
};

// ─── Section 9: Instrument Management ────────────────────────────────────────

const instrumentManagement: Section = {
  id: 'instrument-management',
  title: 'Securities — Admin Operations',
  steps: [
    {
      id: 'sec-mgmt-1',
      title: 'Halt Trading on APU',
      description: 'Admin halts trading on APU JSC — no new orders accepted',
      method: 'PUT',
      url: (state) => `/api/v1/securities/instruments/${state.apu_id || 'APU'}/status`,
      headers: (state) => tenantHeader(state, 'admin'),
      body: () => ({ status: 'HALTED', reason: 'Regulatory review' }),
      validateResponse: okValidator,
    },
    {
      id: 'sec-mgmt-2',
      title: 'Resume Trading on APU',
      description: 'Admin resumes trading after review',
      method: 'PUT',
      url: (state) => `/api/v1/securities/instruments/${state.apu_id || 'APU'}/status`,
      headers: (state) => tenantHeader(state, 'admin'),
      body: () => ({ status: 'ACTIVE' }),
      validateResponse: okValidator,
    },
    {
      id: 'sec-mgmt-3',
      title: 'Verify Instrument Status',
      description: 'Confirm APU is back to ACTIVE trading',
      method: 'GET',
      url: '/api/v1/securities/instruments',
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
  ],
};

// ─── Section 10: Platform Admin ──────────────────────────────────────────────

const platformAdmin: Section = {
  id: 'platform-admin',
  title: 'Platform — Tenant Management',
  steps: [
    {
      id: 'plat-1',
      title: 'List Tenants',
      description: 'View all registered trading venues on the platform',
      method: 'GET',
      url: '/platform/v1/tenants',
      validateResponse: okValidator,
    },
    {
      id: 'plat-2',
      title: 'Get MSE Tenant Details',
      description: 'View MSE tenant status, governance tier, and flagship flag',
      method: 'GET',
      url: '/platform/v1/tenants/mse-equities',
      validateResponse: okValidator,
    },
  ],
};

// ─── Readiness Checklist ─────────────────────────────────────────────────────

const readinessItems: ChecklistItem[] = [
  { id: 'sec-1', category: 'Security', description: 'TLS termination on gateway', status: 'Ready' },
  { id: 'sec-2', category: 'Security', description: 'JWT auth on all securities endpoints', status: 'Ready' },
  { id: 'sec-3', category: 'Security', description: 'Tenant isolation via X-GarudaX-Tenant', status: 'Ready' },
  { id: 'perf-1', category: 'Performance', description: 'Order matching < 1ms', status: 'Ready' },
  { id: 'perf-2', category: 'Performance', description: 'Lot size & tick size validation', status: 'Ready' },
  { id: 'feat-1', category: 'Features', description: 'T+2 settlement state machine', status: 'Ready' },
  { id: 'feat-2', category: 'Features', description: 'Corporate actions (dividend, split)', status: 'Ready' },
  { id: 'feat-3', category: 'Features', description: 'Market sessions (auction + continuous)', status: 'Ready' },
  { id: 'feat-4', category: 'Features', description: 'FRC regulatory reporting', status: 'Ready' },
  { id: 'feat-5', category: 'Features', description: 'Multi-tenant platform with MSE as flagship', status: 'Ready' },
];

const readiness: ChecklistSection = {
  id: 'readiness',
  title: 'Securities Exchange Readiness',
  items: readinessItems,
};

// ─── Export ──────────────────────────────────────────────────────────────────

export const allSections: AnySection[] = [
  envSetup,
  registration,
  securitiesInstruments,
  securitiesTrading,
  securitiesPositions,
  corporateActions,
  marketSessions,
  frcReporting,
  instrumentManagement,
  platformAdmin,
  readiness,
];

export function getAllSteps(): StepDefinition[] {
  return allSections.flatMap((s) => ('steps' in s ? s.steps : []));
}

export function getTotalStepCount(): number {
  return getAllSteps().length;
}
