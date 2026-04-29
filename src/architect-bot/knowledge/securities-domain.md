# Securities Exchange Domain Knowledge

## Order Types

| Type | Behavior |
|------|----------|
| **LIMIT** | Execute at specified price or better. Rests on the book if no immediate match. |
| **MARKET** | Execute immediately at best available price. No price guarantee. |
| **STOP** | Becomes a market order when stop price is reached. Used for loss protection. |
| **STOP_LIMIT** | Becomes a limit order when stop price is reached. Combines stop protection with price control. |

## Time-in-Force

| TIF | Behavior |
|-----|----------|
| **DAY** | Active until market close. Cancelled automatically at end of trading session. |
| **GTC** | Good Till Cancelled. Remains active across sessions until explicitly cancelled. |
| **IOC** | Immediate Or Cancel. Executes whatever quantity is available immediately; remainder is cancelled. |
| **FOK** | Fill Or Kill. Executes the entire quantity immediately or the entire order is cancelled. No partial fills. |
| **GTD** | Good Till Date. Active until a specified expiry date. |

## Matching Engine Concepts

### Price-Time Priority
The standard matching algorithm for Central Limit Order Books (CLOB):
1. Orders are ranked first by price (best price first: highest bid, lowest ask)
2. At the same price, orders are ranked by arrival time (earliest first)
3. Incoming orders match against the best resting order on the opposite side

### Iceberg Orders
Orders with a visible (displayed) quantity and a hidden (reserve) quantity. Only the visible portion appears on the order book. When the visible portion fills, it is automatically replenished from the hidden reserve. This allows large orders to execute without revealing full intent to the market.

### Self-Trade Prevention (STP)
Prevents a firm from trading against its own orders. Three modes:
- **STP_CANCEL_NEWEST**: Cancel the incoming (aggressor) order
- **STP_CANCEL_OLDEST**: Cancel the resting order
- **STP_CANCEL_BOTH**: Cancel both orders

## Auction Mechanisms

### Opening Auction
Runs during PRE_OPEN phase. Orders accumulate without matching. At auction close, a clearing price is calculated that maximizes matched volume. All matched orders execute at the single clearing price.

### Closing Auction
Runs during CLOSING_AUCTION phase. Same mechanism as opening auction. The clearing price becomes the official closing price for the instrument.

### Clearing Price Algorithm
1. Build a price ladder from all accumulated orders
2. At each price level, calculate cumulative buy volume (at or above) and cumulative sell volume (at or below)
3. The clearing price is the level where matched volume (min of buy, sell) is maximized
4. Ties are broken by minimum surplus (difference between buy and sell volume), then by reference price proximity

## Day Lifecycle State Machine

```
DAY_CLOSED --> DAY_PRE_OPEN --> DAY_TRADING --> DAY_POST_CLOSE --> DAY_CLOSED
```

| State | Activity |
|-------|----------|
| **DAY_CLOSED** | No trading. Maintenance, data reconciliation. |
| **DAY_PRE_OPEN** | Order entry begins. Indicative prices published. Opening auction accumulates orders. |
| **DAY_TRADING** | Continuous matching. All order types active. Circuit breakers monitored. |
| **DAY_POST_CLOSE** | Trading stops. Closing auction executes. Settlement obligations generated. Reports produced. |

Each instrument has its own SessionManager with independently controllable phases. Session extend/shorten API allows operators to adjust trading duration per instrument.

## Settlement

### T+2 Settlement Cycle
Trades executed on day T settle on T+2 (two business days later). This gives time for trade affirmation, netting, and instruction delivery.

### Settlement Lifecycle
```
SETTLE_PENDING --> SETTLE_AFFIRMED --> SETTLE_NETTED --> SETTLE_INSTRUCTED --> SETTLE_SETTLING --> SETTLE_SETTLED
                                                                                              --> SETTLE_FAILED
```

### Key Concepts
- **DVP (Delivery Versus Payment)**: Securities and cash move simultaneously. Eliminates settlement risk.
- **FOP (Free of Payment)**: Securities transfer without corresponding cash movement. Used for internal transfers.
- **Netting**: Offsetting buy and sell obligations between the same counterparties to reduce the number of settlements. A firm that bought 1000 and sold 700 of the same security nets to a single delivery of 300.
- **Novation**: The CCP (Central Counterparty) interposes itself between buyer and seller, becoming the buyer to every seller and the seller to every buyer. Eliminates bilateral counterparty risk.
- **Accrued Interest**: For bonds, the interest accumulated since the last coupon payment date, calculated using the bond's day-count convention (ACT/360, ACT/365, or 30/360).

## Corporate Actions

| Type | Description |
|------|-------------|
| **CA_DIVIDEND** | Cash distribution to shareholders. Has ex-date, record date, payment date. Entitlements calculated per position. |
| **CA_STOCK_SPLIT** | Increases shares outstanding by a ratio (e.g., 2:1). Adjusts price and positions proportionally. |
| **CA_RIGHTS_ISSUE** | Existing shareholders offered new shares at a discount. Creates entitlements with subscription rights. |
| **CA_MERGER** | Two entities combine. May involve share conversion ratios and cash components. |

Corporate action lifecycle: ANNOUNCED -> PROCESSING -> COMPLETED (or CANCELLED).

## Surveillance

### 12 Alert Patterns

| Pattern | Detection |
|---------|-----------|
| **LARGE_TRADE** | Trade size exceeds configurable threshold for the instrument |
| **PRICE_SPIKE** | Price movement exceeds threshold within a time window |
| **WASH_TRADE** | Same beneficial owner on both sides of a trade |
| **VOLUME_ANOMALY** | Volume deviates significantly from historical average |
| **FRONT_RUNNING** | Broker trades ahead of a pending client order |
| **SPOOFING** | Large orders placed and cancelled rapidly to create false impression of demand |
| **LAYERING** | Multiple orders at different price levels to manipulate the order book |
| **INSIDER_TRADING** | Unusual trading activity before material announcements |
| **MARKET_MANIPULATION** | Coordinated activity to artificially influence price |
| **CONCENTRATION** | Single entity accumulates excessive position in an instrument |
| **UNUSUAL_ACTIVITY** | Trading patterns that deviate from the entity's historical behavior |
| **CROSS_MARKET** | Correlated suspicious activity across multiple instruments or markets |

Alert lifecycle: OPEN -> INVESTIGATING -> RESOLVED. Alerts can be linked to formal Investigations with evidence tracking.

## Circuit Breakers

Automatic trading halts triggered by extreme price movements:

- **Static bands**: Percentage limits from the reference price (previous closing price). Example: +/-10% from yesterday's close.
- **Dynamic bands**: Percentage limits from the last traded price. Narrower than static bands. Example: +/-5% from last trade.

When triggered, the instrument enters a cooling-off period (`cooldown_minutes`) before trading resumes. Four trigger types: CB_STATIC_UPPER, CB_STATIC_LOWER, CB_DYNAMIC_UPPER, CB_DYNAMIC_LOWER.

## Trading Parameters

Each instrument can have a TradingParameterSet that bundles:
- Tick table (price increment rules by price band)
- Circuit breaker configuration
- Allowed order types and time-in-force values
- Min/max order size and value limits
- Auction parameters (random end, surplus handling, minimum duration)
- STP mode and short selling rules

## Tick Tables

Price increments vary by price level. A tiered tick table defines bands:

| Price Range | Tick Size |
|-------------|-----------|
| 0.01 - 1.00 | 0.01 |
| 1.01 - 10.00 | 0.05 |
| 10.01 - 100.00 | 0.10 |
| 100.01+ | 0.50 |

Orders with prices that don't conform to the tick table are rejected.

## Lot Sizes

Instruments trade in fixed lot sizes (e.g., lot size 100 means orders must be in multiples of 100 shares). Odd lots may be allowed in specific sessions or for specific order types.

## Market Microstructure Concepts

- **Order Book**: The collection of all resting limit orders for an instrument, organized by price level with bid (buy) and ask (sell) sides
- **Spread**: The difference between the best bid and best ask price
- **Depth**: The total volume available at each price level in the order book
- **Price Level**: An aggregated view showing total quantity and order count at a specific price
- **Market Maker**: A participant obligated to continuously quote two-sided markets (bid and ask) to provide liquidity
- **Last Traded Price**: The price of the most recent trade execution
- **Reference Price**: The official price used as anchor for circuit breakers, typically the previous day's closing price
- **OHLCV**: Open, High, Low, Close, Volume — standard candle data for a time period
