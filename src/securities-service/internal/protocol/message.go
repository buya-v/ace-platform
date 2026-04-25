package protocol

// HeaderSize is the fixed size of every message header in bytes.
const HeaderSize = 14

// Header is the 14-byte prefix on every protocol message.
type Header struct {
	MsgType  uint16
	Length   uint32
	Sequence uint64
}

// Message is the interface that all protocol messages implement.
type Message interface {
	Type() uint16
	Encode() ([]byte, error)
}

// Message type constants.
const (
	MsgLogin           uint16 = 0x0001
	MsgLogout          uint16 = 0x0002
	MsgHeartbeat       uint16 = 0x0003
	MsgNewOrder        uint16 = 0x0010
	MsgCancelOrder     uint16 = 0x0011
	MsgExecutionReport uint16 = 0x0020
	MsgOrderBookUpdate uint16 = 0x0021
	MsgReject          uint16 = 0x0030
)

// PriceScale is the fixed-point multiplier for price encoding.
const PriceScale = 10000

// PriceToFixed converts a floating-point price to fixed-point uint64.
func PriceToFixed(price float64) uint64 {
	return uint64(price * PriceScale)
}

// FixedToPrice converts a fixed-point uint64 back to float64.
func FixedToPrice(fixed uint64) float64 {
	return float64(fixed) / PriceScale
}
