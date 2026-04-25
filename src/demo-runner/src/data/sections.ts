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

// ─── Section 1: Environment & Platform ───────────────────────────────────────

const envSetup: Section = {
  id: 'env-setup',
  title: '1. Environment & Platform',
  steps: [
    {
      id: 'env-1',
      title: 'Check Gateway Health',
      description: 'Verify the API gateway is running',
      method: 'GET',
      url: '/healthz',
      validateResponse: okValidator,
    },
    {
      id: 'env-2',
      title: 'Check Readiness',
      description: 'Verify all backend services are ready',
      method: 'GET',
      url: '/readyz',
      validateResponse: okValidator,
    },
    {
      id: 'env-3',
      title: 'List Platform Tenants',
      description: 'Verify MSE tenant is registered on the platform',
      method: 'GET',
      url: '/platform/v1/tenants',
      validateResponse: okValidator,
    },
  ],
};

// ─── Section 2: User Registration ────────────────────────────────────────────

const registration: Section = {
  id: 'registration',
  title: '2. User Registration & Login',
  steps: [
    {
      id: 'reg-1',
      title: 'Register Trader 1 (Buyer)',
      description: 'Create first trader account',
      method: 'POST',
      url: '/api/v1/auth/register',
      body: () => ({ username: 'trader1', password: 'Tr@der1Pass!', email: 'trader1@garudax.mn', role: 'trader' }),
      validateResponse: (status) => (status === 200 || status === 201 || status === 409) ? 'PASS' : 'FAIL',
    },
    {
      id: 'reg-2',
      title: 'Register Trader 2 (Seller)',
      description: 'Create second trader account',
      method: 'POST',
      url: '/api/v1/auth/register',
      body: () => ({ username: 'trader2', password: 'Tr@der2Pass!', email: 'trader2@garudax.mn', role: 'trader' }),
      validateResponse: (status) => (status === 200 || status === 201 || status === 409) ? 'PASS' : 'FAIL',
    },
    {
      id: 'reg-3',
      title: 'Register Exchange Admin',
      description: 'Create market operations admin',
      method: 'POST',
      url: '/api/v1/auth/register',
      body: () => ({ username: 'admin', password: 'Adm1n@Pass!', email: 'admin@garudax.mn', role: 'admin' }),
      validateResponse: (status) => (status === 200 || status === 201 || status === 409) ? 'PASS' : 'FAIL',
    },
    {
      id: 'reg-4',
      title: 'Login Trader 1',
      description: 'Authenticate and store JWT token',
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
      description: 'Authenticate and store JWT token',
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
      description: 'Authenticate exchange admin',
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

// ─── Section 3: Instrument Listing ───────────────────────────────────────────

const instrumentListing: Section = {
  id: 'instrument-listing',
  title: '3. Instrument Listing',
  steps: [
    {
      id: 'inst-1',
      title: 'List Equity: APU JSC',
      description: 'Admin lists APU JSC (food & beverages) — lot=10, tick=1 MNT',
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
      id: 'inst-2',
      title: 'List Equity: Govisumber Mining',
      description: 'Admin lists Govisumber Mining — lot=100, tick=50 MNT',
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
      id: 'inst-3',
      title: 'List Bond: Government Bond 2028',
      description: 'Admin lists a government bond — 8.5% coupon, semi-annual, 100K MNT par value',
      method: 'POST',
      url: '/api/v1/securities/bonds',
      headers: (state) => tenantHeader(state, 'admin'),
      body: () => ({
        instrument_id: 'GOV-BOND-2028',
        maturity_date: '2028-12-31',
        coupon_rate: 8.5,
        coupon_frequency: 'SEMI_ANNUAL',
        par_value: 100000,
        day_count_convention: 'ACT_365',
      }),
      validateResponse: okValidator,
    },
    {
      id: 'inst-4',
      title: 'Calculate Accrued Interest',
      description: 'Calculate accrued interest on the government bond for settlement today',
      method: 'GET',
      url: `/api/v1/securities/bonds/GOV-BOND-2028/accrued-interest?settlement_date=${new Date().toISOString().slice(0, 10)}`,
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'inst-5',
      title: 'Verify All Listings',
      description: 'Confirm equities and bonds are listed on MSE',
      method: 'GET',
      url: '/api/v1/securities/instruments',
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
  ],
};

// ─── Section 4: Start Trading Day ────────────────────────────────────────────

const startDay: Section = {
  id: 'start-day',
  title: '4. Start Trading Day',
  steps: [
    {
      id: 'day-1',
      title: 'Check Day State (should be CLOSED)',
      description: 'Before starting — day state is CLOSED, no trading allowed',
      method: 'GET',
      url: '/api/v1/securities/day/status',
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'day-2',
      title: '▶ Start Day → PRE_OPEN',
      description: 'Admin starts the trading day — all instruments transition to PRE_OPEN phase. Orders are collected but NOT matched.',
      method: 'POST',
      url: '/api/v1/securities/day/start',
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'day-3',
      title: 'Verify Day State = PRE_OPEN',
      description: 'Day is now in PRE_OPEN — orders collected for opening auction',
      method: 'GET',
      url: '/api/v1/securities/day/status',
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'day-4',
      title: 'View Instrument Sessions',
      description: 'All instruments should be in PRE_OPEN session',
      method: 'GET',
      url: '/api/v1/securities/sessions',
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
  ],
};

// ─── Section 5: Pre-Open Auction Orders ──────────────────────────────────────

const preOpenOrders: Section = {
  id: 'pre-open-orders',
  title: '5. Pre-Open Auction Orders',
  steps: [
    {
      id: 'pre-1',
      title: 'Buyer: Submit Pre-Open Buy @ 850',
      description: 'Trader 1 submits buy order during PRE_OPEN — order is COLLECTED, not matched',
      method: 'POST',
      url: '/api/v1/securities/orders',
      headers: (state) => tenantHeader(state, 'trader1'),
      body: (state) => ({
        instrument_id: state.apu_id || 'APU',
        side: 'BUY',
        order_type: 'LIMIT',
        quantity: 100,
        price: 850,
      }),
      validateResponse: okValidator,
    },
    {
      id: 'pre-2',
      title: 'Buyer: Submit Pre-Open Buy @ 855',
      description: 'Another buy at higher price — will compete in auction',
      method: 'POST',
      url: '/api/v1/securities/orders',
      headers: (state) => tenantHeader(state, 'trader1'),
      body: (state) => ({
        instrument_id: state.apu_id || 'APU',
        side: 'BUY',
        order_type: 'LIMIT',
        quantity: 50,
        price: 855,
      }),
      validateResponse: okValidator,
    },
    {
      id: 'pre-3',
      title: 'Seller: Submit Pre-Open Sell @ 845',
      description: 'Trader 2 submits sell — these orders will match in the opening auction',
      method: 'POST',
      url: '/api/v1/securities/orders',
      headers: (state) => tenantHeader(state, 'trader2'),
      body: (state) => ({
        instrument_id: state.apu_id || 'APU',
        side: 'SELL',
        order_type: 'LIMIT',
        quantity: 80,
        price: 845,
      }),
      validateResponse: okValidator,
    },
    {
      id: 'pre-4',
      title: 'Seller: Submit Pre-Open Sell @ 850',
      description: 'Another sell order for the auction',
      method: 'POST',
      url: '/api/v1/securities/orders',
      headers: (state) => tenantHeader(state, 'trader2'),
      body: (state) => ({
        instrument_id: state.apu_id || 'APU',
        side: 'SELL',
        order_type: 'LIMIT',
        quantity: 70,
        price: 850,
      }),
      validateResponse: okValidator,
    },
    {
      id: 'pre-5',
      title: 'Verify: No Trades Yet',
      description: 'Orders are collected but NO trades executed during PRE_OPEN',
      method: 'GET',
      url: '/api/v1/securities/orders',
      headers: (state) => tenantHeader(state, 'trader1'),
      validateResponse: okValidator,
    },
  ],
};

// ─── Section 6: Opening Auction + Continuous Trading ─────────────────────────

const startTrading: Section = {
  id: 'start-trading',
  title: '6. Opening Auction → Continuous Trading',
  steps: [
    {
      id: 'trade-1',
      title: '▶ Start Trading → Opening Auction Executes',
      description: 'Admin starts trading — opening auction runs at clearing price, then CONTINUOUS matching begins. Pre-open orders are matched!',
      method: 'POST',
      url: '/api/v1/securities/day/trading',
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'trade-2',
      title: 'Verify Day State = TRADING',
      description: 'Day is now in TRADING state — continuous matching active',
      method: 'GET',
      url: '/api/v1/securities/day/status',
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'trade-3',
      title: 'View Orders (some should be FILLED)',
      description: 'Pre-open orders that crossed in the auction are now FILLED',
      method: 'GET',
      url: '/api/v1/securities/orders',
      headers: (state) => tenantHeader(state, 'trader1'),
      validateResponse: okValidator,
    },
    {
      id: 'trade-4',
      title: 'Submit Continuous Buy Order',
      description: 'Trader 1 submits during continuous trading — matches immediately if price crosses',
      method: 'POST',
      url: '/api/v1/securities/orders',
      headers: (state) => tenantHeader(state, 'trader1'),
      body: (state) => ({
        instrument_id: state.apu_id || 'APU',
        side: 'BUY',
        order_type: 'LIMIT',
        quantity: 30,
        price: 860,
      }),
      validateResponse: okValidator,
    },
    {
      id: 'trade-5',
      title: 'Submit Matching Sell Order',
      description: 'Trader 2 sells at 860 — instant match, trade executed',
      method: 'POST',
      url: '/api/v1/securities/orders',
      headers: (state) => tenantHeader(state, 'trader2'),
      body: (state) => ({
        instrument_id: state.apu_id || 'APU',
        side: 'SELL',
        order_type: 'LIMIT',
        quantity: 30,
        price: 860,
      }),
      validateResponse: okValidator,
    },
  ],
};

// ─── Section 7: End Trading Day ──────────────────────────────────────────────

const endDay: Section = {
  id: 'end-day',
  title: '7. Close Market → End Day',
  steps: [
    {
      id: 'close-1',
      title: '▶ End Trading → Closing Auction',
      description: 'Admin ends trading — closing auction executes for remaining orders, then instruments close',
      method: 'POST',
      url: '/api/v1/securities/day/end-trading',
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'close-2',
      title: 'Verify Day State = POST_CLOSE',
      description: 'Trading has ended — post-trade processing in progress',
      method: 'GET',
      url: '/api/v1/securities/day/status',
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'close-3',
      title: '▶ End Day → CLOSED',
      description: 'Admin closes the trading day — all processing complete',
      method: 'POST',
      url: '/api/v1/securities/day/end',
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'close-4',
      title: 'Verify Day State = CLOSED',
      description: 'Trading day is complete — ready for next business day',
      method: 'GET',
      url: '/api/v1/securities/day/status',
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
  ],
};

// ─── Section 8: Post-Trade — Settlement & Positions ──────────────────────────

const postTrade: Section = {
  id: 'post-trade',
  title: '8. Post-Trade — Settlement & Reporting',
  steps: [
    {
      id: 'post-1',
      title: 'View All Orders',
      description: 'Review all orders from today — auction fills + continuous fills',
      method: 'GET',
      url: '/api/v1/securities/orders',
      headers: (state) => tenantHeader(state, 'trader1'),
      validateResponse: okValidator,
    },
    {
      id: 'post-2',
      title: 'View Settlement Obligations',
      description: 'T+2 settlement obligations created from today\'s trades',
      method: 'GET',
      url: '/api/v1/securities/settlements?status=PENDING',
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
    {
      id: 'post-3',
      title: 'Trigger Settlement Cycle',
      description: 'Process settlement — PENDING → AFFIRMED → NETTED → SETTLED',
      method: 'POST',
      url: '/api/v1/securities/settlements/cycle',
      headers: (state) => tenantHeader(state, 'admin'),
      body: () => ({ date: new Date().toISOString().slice(0, 10) }),
      validateResponse: okValidator,
    },
    {
      id: 'post-4',
      title: 'FRC Daily Trading Summary',
      description: 'Generate regulatory report — trade count, volume, value',
      method: 'GET',
      url: `/api/v1/securities/reports/frc?type=DAILY_SUMMARY&date=${new Date().toISOString().slice(0, 10)}`,
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
  ],
};

// ─── Section 9: Corporate Actions ────────────────────────────────────────────

const corporateActions: Section = {
  id: 'corporate-actions',
  title: '9. Corporate Actions',
  steps: [
    {
      id: 'ca-1',
      title: 'Announce Dividend: APU 50 MNT/share',
      description: 'APU JSC declares 50 MNT per share cash dividend',
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
      id: 'ca-2',
      title: 'Process Dividend Entitlements',
      description: 'Calculate dividend payments for all APU shareholders',
      method: 'POST',
      url: (state) => `/api/v1/securities/corporate-actions/${state.dividend_id || 'DIV-001'}/process`,
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
  ],
};

// ─── Section 10: Admin Operations ────────────────────────────────────────────

const adminOps: Section = {
  id: 'admin-ops',
  title: '10. Admin Operations',
  steps: [
    {
      id: 'admin-1',
      title: 'Halt Trading on APU',
      description: 'Emergency halt — no new orders accepted',
      method: 'PUT',
      url: (state) => `/api/v1/securities/instruments/${state.apu_id || 'APU'}/status`,
      headers: (state) => tenantHeader(state, 'admin'),
      body: () => ({ status: 'HALTED', reason: 'Regulatory review' }),
      validateResponse: okValidator,
    },
    {
      id: 'admin-2',
      title: 'Resume Trading on APU',
      description: 'Regulatory review complete — trading resumes',
      method: 'PUT',
      url: (state) => `/api/v1/securities/instruments/${state.apu_id || 'APU'}/status`,
      headers: (state) => tenantHeader(state, 'admin'),
      body: () => ({ status: 'ACTIVE' }),
      validateResponse: okValidator,
    },
    {
      id: 'admin-3',
      title: 'Verify Instruments',
      description: 'Confirm APU is back to ACTIVE',
      method: 'GET',
      url: '/api/v1/securities/instruments',
      headers: (state) => tenantHeader(state, 'admin'),
      validateResponse: okValidator,
    },
  ],
};

// ─── Readiness Checklist ─────────────────────────────────────────────────────

const readinessItems: ChecklistItem[] = [
  { id: 'sec-1', category: 'Day Lifecycle', description: 'Start Day → PRE_OPEN → Trading → POST_CLOSE → End Day', status: 'Ready' },
  { id: 'sec-2', category: 'Day Lifecycle', description: 'Opening auction executes on PRE_OPEN → CONTINUOUS transition', status: 'Ready' },
  { id: 'sec-3', category: 'Day Lifecycle', description: 'Closing auction executes on CONTINUOUS → CLOSED transition', status: 'Ready' },
  { id: 'feat-1', category: 'Trading', description: 'Price-time priority continuous matching', status: 'Ready' },
  { id: 'feat-2', category: 'Trading', description: 'Lot size & tick size validation', status: 'Ready' },
  { id: 'feat-3', category: 'Trading', description: 'Circuit breakers (static + dynamic)', status: 'Ready' },
  { id: 'feat-4', category: 'Trading', description: 'Self-trade prevention', status: 'Ready' },
  { id: 'feat-5', category: 'Settlement', description: 'T+2 settlement state machine', status: 'Ready' },
  { id: 'feat-6', category: 'Settlement', description: 'Corporate actions (dividend, split)', status: 'Ready' },
  { id: 'feat-7', category: 'Regulatory', description: 'FRC daily trading summary report', status: 'Ready' },
  { id: 'feat-8', category: 'Platform', description: 'Multi-tenant with MSE as flagship', status: 'Ready' },
  { id: 'feat-9', category: 'Connectivity', description: 'FIX 4.4 protocol gateway', status: 'Ready' },
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
  instrumentListing,
  startDay,
  preOpenOrders,
  startTrading,
  endDay,
  postTrade,
  corporateActions,
  adminOps,
  readiness,
];

export function getAllSteps(): StepDefinition[] {
  return allSections.flatMap((s) => ('steps' in s ? s.steps : []));
}

export function getTotalStepCount(): number {
  return getAllSteps().length;
}
