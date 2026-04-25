# GarudaX Native Binary Protocol Specification

**Version:** 1.0
**Status:** Draft
**Date:** 2026-04-25

## 1. Overview

The GarudaX Native Binary Protocol is a low-latency, fixed-field binary protocol
for order entry and execution reporting between trading participants and the
GarudaX matching engine. It is designed for direct TCP connections where FIX
protocol overhead is undesirable.

All multi-byte integers are encoded **big-endian** (network byte order).
All string fields are **fixed-length**, right-padded with ASCII space (0x20).

## 2. Transport

- **Layer:** TCP
- **Framing:** Length-prefixed (the `Length` field in the header includes the header itself)
- **Byte order:** Big-endian for all numeric fields
- **Character encoding:** ASCII for string fields

## 3. Message Header

Every message begins with a 14-byte header:

```
Offset  Size  Type    Field     Description
------  ----  ------  --------  -----------
0       2     uint16  MsgType   Message type identifier
2       4     uint32  Length    Total message length including header (bytes)
6       8     uint64  Sequence  Sender-assigned sequence number
```

**Header size: 14 bytes**

The `Length` field is the total size of the message (header + payload).
A heartbeat message with no payload has `Length = 14`.

## 4. Message Types

| Code   | Name             | Direction        | Description                        |
|--------|------------------|------------------|------------------------------------|
| 0x0001 | Login            | Client → Server  | Session authentication             |
| 0x0002 | Logout           | Bidirectional    | Graceful session termination       |
| 0x0003 | Heartbeat        | Bidirectional    | Keep-alive, no payload             |
| 0x0010 | NewOrder         | Client → Server  | Submit a new order                 |
| 0x0011 | CancelOrder      | Client → Server  | Cancel an existing order           |
| 0x0020 | ExecutionReport  | Server → Client  | Order status update / fill         |
| 0x0021 | OrderBookUpdate  | Server → Client  | Market data book change (reserved) |
| 0x0030 | Reject           | Server → Client  | Session-level rejection            |

## 5. Field Encoding Rules

### 5.1 Numeric Fields

All numeric fields use big-endian byte order:
- `uint16` — 2 bytes
- `uint32` — 4 bytes
- `uint64` — 8 bytes
- `byte`   — 1 byte (unsigned)

### 5.2 String Fields

String fields are fixed-length byte arrays. Values shorter than the field
width are **right-padded with ASCII space** (0x20). Values must not exceed
the field width.

Example: CompID `"BROKER01"` in a `[12]byte` field:
```
42 52 4F 4B 45 52 30 31 20 20 20 20
B  R  O  K  E  R  0  1  SP SP SP SP
```

### 5.3 Price Encoding

Prices are encoded as `uint64` using fixed-point arithmetic with a
**scale factor of 10,000** (4 implied decimal places).

| Price    | Encoded Value |
|----------|---------------|
| 100.00   | 1,000,000     |
| 850.25   | 8,502,500     |
| 0.0001   | 1             |
| 99999.99 | 999,999,900   |

Conversion:
```
encoded = uint64(price * 10000)
price   = float64(encoded) / 10000.0
```

### 5.4 Side Values

| Value | Meaning |
|-------|---------|
| 'B'   | Buy     |
| 'S'   | Sell    |

### 5.5 Order Type Values

| Value | Meaning      |
|-------|-------------|
| 'L'   | Limit       |
| 'M'   | Market      |
| 'S'   | Stop        |

### 5.6 Time In Force Values

| Value | Meaning               |
|-------|-----------------------|
| '0'   | Day                   |
| '1'   | Good Till Cancel (GTC)|
| '2'   | Immediate or Cancel   |
| '3'   | Fill or Kill          |

### 5.7 Execution Type Values

| Value | Meaning          |
|-------|-----------------|
| '0'   | New              |
| '1'   | Partial Fill     |
| '2'   | Fill             |
| '3'   | Done for Day     |
| '4'   | Cancelled        |
| '5'   | Replaced         |
| '8'   | Rejected         |

### 5.8 Order Status Values

| Value | Meaning          |
|-------|-----------------|
| '0'   | New              |
| '1'   | Partially Filled |
| '2'   | Filled           |
| '3'   | Done for Day     |
| '4'   | Cancelled        |
| '5'   | Replaced         |
| '8'   | Rejected         |

## 6. Message Definitions

### 6.1 Login (0x0001)

Sent by the client to authenticate and establish a session.

```
Offset  Size  Type      Field     Description
------  ----  --------  --------  -----------
14      12    [12]byte  CompID    Sender component identifier
26      20    [20]byte  Password  Authentication credential
```

**Total message size:** 14 (header) + 32 (payload) = 46 bytes

The server responds with either a Heartbeat (login accepted) or
a Reject message (login denied).

### 6.2 Logout (0x0002)

Sent by either side to terminate the session gracefully.

```
Offset  Size  Type      Field   Description
------  ----  --------  ------  -----------
14      32    [32]byte  Reason  Human-readable logout reason
```

**Total message size:** 14 + 32 = 46 bytes

### 6.3 Heartbeat (0x0003)

Sent by both sides to maintain session liveness. No payload.

**Total message size:** 14 bytes (header only)

### 6.4 NewOrder (0x0010)

Submit a new order for matching.

```
Offset  Size  Type      Field          Description
------  ----  --------  -------------  -----------
14      12    [12]byte  InstrumentID   Instrument identifier
26      1     byte      Side           'B' = Buy, 'S' = Sell
27      1     byte      OrderType      'L' = Limit, 'M' = Market, 'S' = Stop
28      4     uint32    Quantity       Order quantity
32      8     uint64    Price          Price (fixed-point × 10,000)
40      1     byte      TimeInForce    Time-in-force code
41      20    [20]byte  ClientOrderID  Client-assigned order identifier
```

**Total message size:** 14 + 47 = 61 bytes

### 6.5 CancelOrder (0x0011)

Cancel a previously submitted order.

```
Offset  Size  Type      Field              Description
------  ----  --------  -----------------  -----------
14      20    [20]byte  OrigClientOrderID  Original client order ID to cancel
34      12    [12]byte  InstrumentID       Instrument identifier
```

**Total message size:** 14 + 32 = 46 bytes

### 6.6 ExecutionReport (0x0020)

Server-sent notification of order status change or execution.

```
Offset  Size  Type      Field      Description
------  ----  --------  ---------  -----------
14      20    [20]byte  OrderID    Exchange-assigned order identifier
34      20    [20]byte  ExecID     Execution identifier
54      1     byte      ExecType   Execution type (see §5.7)
55      1     byte      OrdStatus  Order status (see §5.8)
56      1     byte      Side       'B' = Buy, 'S' = Sell
57      4     uint32    Quantity   Original order quantity
61      8     uint64    Price      Execution price (fixed-point × 10,000)
69      4     uint32    LeavesQty  Remaining open quantity
73      4     uint32    CumQty     Cumulative filled quantity
```

**Total message size:** 14 + 63 = 77 bytes

### 6.7 OrderBookUpdate (0x0021)

Reserved for future market data distribution. Specification TBD.

### 6.8 Reject (0x0030)

Session-level rejection of a message.

```
Offset  Size  Type      Field      Description
------  ----  --------  ---------  -----------
14      2     uint16    RefMsgType Message type of the rejected message
16      64    [64]byte  Reason     Human-readable rejection reason
```

**Total message size:** 14 + 66 = 80 bytes

## 7. Session Management

### 7.1 Connection Establishment

1. Client opens TCP connection to the server
2. Client sends a Login message (sequence = 1)
3. Server authenticates using CompID + Password
4. On success: server sends Heartbeat (sequence = 1)
5. On failure: server sends Reject and closes the connection

### 7.2 Sequence Numbers

- Each direction maintains an independent, monotonically increasing sequence
  counter starting at 1
- The sender increments the sequence for every message sent
- The receiver tracks the expected sequence; a gap indicates message loss
- Sequence numbers wrap at `2^64 - 1` (effectively never in practice)

### 7.3 Heartbeat

- Both sides send Heartbeat messages at a **30-second interval** when idle
- If no message (including heartbeats) is received within **3 × heartbeat
  interval** (90 seconds), the connection is considered dead and should be
  terminated
- Heartbeats carry the current sender sequence number

### 7.4 Logout

Either side may initiate logout by sending a Logout message. The receiving
side should respond with its own Logout message before closing the connection.

### 7.5 Reconnection

After disconnection, the client should:
1. Open a new TCP connection
2. Send Login with sequence = 1 (sequences reset per session)
3. Re-submit any unacknowledged orders

## 8. Error Handling

- Unknown message types: server sends Reject with RefMsgType set to the
  unknown type code and closes the connection
- Malformed messages (truncated, invalid field values): server sends Reject
- Sequence gaps: logged but not automatically recovered (application-level
  concern)

## 9. Wire Format Summary

```
+--------+--------+----------+-------------------+
| MsgType| Length | Sequence |     Payload       |
| 2 bytes| 4 bytes| 8 bytes  | (Length-14) bytes |
+--------+--------+----------+-------------------+
|<-------- Header (14 bytes) -------->|
|<------------- Length bytes ------------------>|
```

## 10. Revision History

| Version | Date       | Author    | Changes            |
|---------|------------|-----------|--------------------|
| 1.0     | 2026-04-25 | GarudaX   | Initial draft      |
