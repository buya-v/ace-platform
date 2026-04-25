package protocol

import (
	"encoding/binary"
)

// LoginMsg authenticates a session. CompID and Password are fixed-length,
// right-padded with spaces.
type LoginMsg struct {
	CompID   [12]byte
	Password [20]byte
}

func (m *LoginMsg) Type() uint16 { return MsgLogin }

func (m *LoginMsg) Encode() ([]byte, error) {
	buf := make([]byte, 32)
	copy(buf[0:12], m.CompID[:])
	copy(buf[12:32], m.Password[:])
	return buf, nil
}

// LogoutMsg terminates a session with an optional reason.
type LogoutMsg struct {
	Reason [32]byte
}

func (m *LogoutMsg) Type() uint16 { return MsgLogout }

func (m *LogoutMsg) Encode() ([]byte, error) {
	buf := make([]byte, 32)
	copy(buf[0:32], m.Reason[:])
	return buf, nil
}

// HeartbeatMsg is a keep-alive with no payload.
type HeartbeatMsg struct{}

func (m *HeartbeatMsg) Type() uint16 { return MsgHeartbeat }

func (m *HeartbeatMsg) Encode() ([]byte, error) {
	return nil, nil
}

// NewOrderMsg submits a new order to the matching engine.
type NewOrderMsg struct {
	InstrumentID  [12]byte
	Side          byte // 'B' or 'S'
	OrderType     byte // 'L', 'M', or 'S'
	Quantity      uint32
	Price         uint64 // fixed-point × 10,000
	TimeInForce   byte
	ClientOrderID [20]byte
}

func (m *NewOrderMsg) Type() uint16 { return MsgNewOrder }

func (m *NewOrderMsg) Encode() ([]byte, error) {
	buf := make([]byte, 47)
	copy(buf[0:12], m.InstrumentID[:])
	buf[12] = m.Side
	buf[13] = m.OrderType
	binary.BigEndian.PutUint32(buf[14:18], m.Quantity)
	binary.BigEndian.PutUint64(buf[18:26], m.Price)
	buf[26] = m.TimeInForce
	copy(buf[27:47], m.ClientOrderID[:])
	return buf, nil
}

// CancelOrderMsg cancels a previously submitted order.
type CancelOrderMsg struct {
	OrigClientOrderID [20]byte
	InstrumentID      [12]byte
}

func (m *CancelOrderMsg) Type() uint16 { return MsgCancelOrder }

func (m *CancelOrderMsg) Encode() ([]byte, error) {
	buf := make([]byte, 32)
	copy(buf[0:20], m.OrigClientOrderID[:])
	copy(buf[20:32], m.InstrumentID[:])
	return buf, nil
}

// ExecutionReportMsg reports order status changes and fills.
type ExecutionReportMsg struct {
	OrderID   [20]byte
	ExecID    [20]byte
	ExecType  byte
	OrdStatus byte
	Side      byte
	Quantity  uint32
	Price     uint64 // fixed-point × 10,000
	LeavesQty uint32
	CumQty    uint32
}

func (m *ExecutionReportMsg) Type() uint16 { return MsgExecutionReport }

func (m *ExecutionReportMsg) Encode() ([]byte, error) {
	buf := make([]byte, 63)
	copy(buf[0:20], m.OrderID[:])
	copy(buf[20:40], m.ExecID[:])
	buf[40] = m.ExecType
	buf[41] = m.OrdStatus
	buf[42] = m.Side
	binary.BigEndian.PutUint32(buf[43:47], m.Quantity)
	binary.BigEndian.PutUint64(buf[47:55], m.Price)
	binary.BigEndian.PutUint32(buf[55:59], m.LeavesQty)
	binary.BigEndian.PutUint32(buf[59:63], m.CumQty)
	return buf, nil
}

// RejectMsg is a session-level rejection of a received message.
type RejectMsg struct {
	RefMsgType uint16
	Reason     [64]byte
}

func (m *RejectMsg) Type() uint16 { return MsgReject }

func (m *RejectMsg) Encode() ([]byte, error) {
	buf := make([]byte, 66)
	binary.BigEndian.PutUint16(buf[0:2], m.RefMsgType)
	copy(buf[2:66], m.Reason[:])
	return buf, nil
}
