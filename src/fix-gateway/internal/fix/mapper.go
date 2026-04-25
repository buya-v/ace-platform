package fix

import (
	"fmt"
	"strconv"
	"time"
)

// Side constants (internal representation).
const (
	SideBuy       = "BUY"
	SideSell      = "SELL"
	SideShortSell = "SHORT_SELL"
)

// OrdType constants (internal representation).
const (
	OrdTypeMarket    = "MARKET"
	OrdTypeLimit     = "LIMIT"
	OrdTypeStop      = "STOP"
	OrdTypeStopLimit = "STOP_LIMIT"
)

// TimeInForce constants (internal representation).
const (
	TIFDAY = "DAY"
	TIFGTC = "GTC"
	TIFIOC = "IOC"
	TIFFOK = "FOK"
	TIFGTD = "GTD"
)

// InternalOrder represents a parsed order from a FIX NewOrderSingle message.
type InternalOrder struct {
	InstrumentID  string
	Side          string
	OrderType     string
	Quantity      int
	Price         float64
	StopPrice     float64
	TimeInForce   string
	ClientOrderID string
	Account       string
	IsShortSell   bool
	TransactTime  string
}

// MapNewOrderSingle extracts an InternalOrder from a parsed FIX NewOrderSingle message.
func MapNewOrderSingle(msg *FIXMessage) (*InternalOrder, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil message")
	}

	// Validate message type.
	msgType := GetTag(msg, TagMsgType)
	if msgType != "" && msgType != MsgTypeNewOrderSingle {
		return nil, fmt.Errorf("expected MsgType=%s, got %s", MsgTypeNewOrderSingle, msgType)
	}

	// Required fields.
	clOrdID := GetTag(msg, TagClOrdID)
	if clOrdID == "" {
		return nil, fmt.Errorf("missing required tag ClOrdID(%d)", TagClOrdID)
	}

	symbol := GetTag(msg, TagSymbol)
	if symbol == "" {
		return nil, fmt.Errorf("missing required tag Symbol(%d)", TagSymbol)
	}

	sideStr := GetTag(msg, TagSide)
	if sideStr == "" {
		return nil, fmt.Errorf("missing required tag Side(%d)", TagSide)
	}
	side, isShort, err := mapSide(sideStr)
	if err != nil {
		return nil, err
	}

	ordTypeStr := GetTag(msg, TagOrdType)
	if ordTypeStr == "" {
		return nil, fmt.Errorf("missing required tag OrdType(%d)", TagOrdType)
	}
	ordType, err := mapOrdType(ordTypeStr)
	if err != nil {
		return nil, err
	}

	tifStr := GetTag(msg, TagTimeInForce)
	if tifStr == "" {
		return nil, fmt.Errorf("missing required tag TimeInForce(%d)", TagTimeInForce)
	}
	tif, err := mapTimeInForce(tifStr)
	if err != nil {
		return nil, err
	}

	qty := GetIntTag(msg, TagOrderQty)
	if qty <= 0 {
		return nil, fmt.Errorf("invalid OrderQty: %d", qty)
	}

	order := &InternalOrder{
		InstrumentID:  symbol,
		Side:          side,
		OrderType:     ordType,
		Quantity:      qty,
		Price:         GetFloatTag(msg, TagPrice),
		StopPrice:     GetFloatTag(msg, TagStopPx),
		TimeInForce:   tif,
		ClientOrderID: clOrdID,
		Account:       GetTag(msg, TagAccount),
		IsShortSell:   isShort,
		TransactTime:  GetTag(msg, TagTransactTime),
	}

	return order, nil
}

// MapExecutionReport builds a FIX ExecutionReport message from internal order data.
func MapExecutionReport(orderID, execID, execType, ordStatus, side string, qty int, price float64, leavesQty, cumQty int) *FIXMessage {
	msg := &FIXMessage{
		Fields: map[int]string{
			TagMsgType:   MsgTypeExecutionReport,
			TagOrderID:   orderID,
			TagExecID:    execID,
			TagExecType:  execType,
			TagOrdStatus: ordStatus,
			TagSide:      mapSideToFIX(side),
			TagOrderQty:  strconv.Itoa(qty),
			TagLeavesQty: strconv.Itoa(leavesQty),
			TagCumQty:    strconv.Itoa(cumQty),
			TagAvgPx:     formatPrice(price),
			TagTransactTime: time.Now().UTC().Format("20060102-15:04:05.000"),
		},
	}
	return msg
}

// mapSide converts FIX side value to internal side and short-sell flag.
func mapSide(fixSide string) (string, bool, error) {
	switch fixSide {
	case "1":
		return SideBuy, false, nil
	case "2":
		return SideSell, false, nil
	case "5":
		return SideShortSell, true, nil
	case "6":
		return SideShortSell, true, nil
	default:
		return "", false, fmt.Errorf("unknown Side value: %s", fixSide)
	}
}

// mapSideToFIX converts internal side to FIX side value.
func mapSideToFIX(side string) string {
	switch side {
	case SideBuy:
		return "1"
	case SideSell:
		return "2"
	case SideShortSell:
		return "5"
	default:
		return "1"
	}
}

// mapOrdType converts FIX order type to internal order type.
func mapOrdType(fixOrdType string) (string, error) {
	switch fixOrdType {
	case "1":
		return OrdTypeMarket, nil
	case "2":
		return OrdTypeLimit, nil
	case "3":
		return OrdTypeStop, nil
	case "4":
		return OrdTypeStopLimit, nil
	default:
		return "", fmt.Errorf("unknown OrdType value: %s", fixOrdType)
	}
}

// mapTimeInForce converts FIX TIF value to internal TIF.
func mapTimeInForce(fixTIF string) (string, error) {
	switch fixTIF {
	case "0":
		return TIFDAY, nil
	case "1":
		return TIFGTC, nil
	case "3":
		return TIFIOC, nil
	case "4":
		return TIFFOK, nil
	case "6":
		return TIFGTD, nil
	default:
		return "", fmt.Errorf("unknown TimeInForce value: %s", fixTIF)
	}
}

func formatPrice(price float64) string {
	return strconv.FormatFloat(price, 'f', 4, 64)
}
