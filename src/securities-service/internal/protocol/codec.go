package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Encoder writes protocol messages to an io.Writer.
type Encoder struct {
	w   io.Writer
	seq uint64
}

// NewEncoder creates an Encoder that writes to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Encode writes a complete message (header + payload) to the writer.
// It auto-increments the sequence number for each message sent.
func (e *Encoder) Encode(msg Message) error {
	payload, err := msg.Encode()
	if err != nil {
		return fmt.Errorf("encode payload: %w", err)
	}

	e.seq++
	totalLen := uint32(HeaderSize + len(payload))

	// Write header
	hdr := make([]byte, HeaderSize)
	binary.BigEndian.PutUint16(hdr[0:2], msg.Type())
	binary.BigEndian.PutUint32(hdr[2:6], totalLen)
	binary.BigEndian.PutUint64(hdr[6:14], e.seq)

	if _, err := e.w.Write(hdr); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	if len(payload) > 0 {
		if _, err := e.w.Write(payload); err != nil {
			return fmt.Errorf("write payload: %w", err)
		}
	}

	return nil
}

// Decoder reads protocol messages from an io.Reader.
type Decoder struct {
	r io.Reader
}

// NewDecoder creates a Decoder that reads from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: r}
}

// Decode reads the next message from the reader. It returns the header
// and the decoded message. An error is returned for truncated reads,
// unknown message types, or payload decode failures.
func (d *Decoder) Decode() (*Header, Message, error) {
	// Read header
	hdrBuf := make([]byte, HeaderSize)
	if _, err := io.ReadFull(d.r, hdrBuf); err != nil {
		return nil, nil, fmt.Errorf("read header: %w", err)
	}

	hdr := &Header{
		MsgType:  binary.BigEndian.Uint16(hdrBuf[0:2]),
		Length:   binary.BigEndian.Uint32(hdrBuf[2:6]),
		Sequence: binary.BigEndian.Uint64(hdrBuf[6:14]),
	}

	payloadLen := int(hdr.Length) - HeaderSize
	if payloadLen < 0 {
		return nil, nil, fmt.Errorf("invalid length %d: less than header size", hdr.Length)
	}

	var payload []byte
	if payloadLen > 0 {
		payload = make([]byte, payloadLen)
		if _, err := io.ReadFull(d.r, payload); err != nil {
			return nil, nil, fmt.Errorf("read payload: %w", err)
		}
	}

	msg, err := decodePayload(hdr.MsgType, payload)
	if err != nil {
		return hdr, nil, err
	}

	return hdr, msg, nil
}

// decodePayload dispatches by message type to decode the payload bytes
// into a concrete Message implementation.
func decodePayload(msgType uint16, payload []byte) (Message, error) {
	switch msgType {
	case MsgLogin:
		return decodeLogin(payload)
	case MsgLogout:
		return decodeLogout(payload)
	case MsgHeartbeat:
		return &HeartbeatMsg{}, nil
	case MsgNewOrder:
		return decodeNewOrder(payload)
	case MsgCancelOrder:
		return decodeCancelOrder(payload)
	case MsgExecutionReport:
		return decodeExecutionReport(payload)
	case MsgReject:
		return decodeReject(payload)
	default:
		return nil, fmt.Errorf("unknown message type: 0x%04X", msgType)
	}
}

func decodeLogin(p []byte) (*LoginMsg, error) {
	if len(p) < 32 {
		return nil, fmt.Errorf("login payload too short: %d < 32", len(p))
	}
	msg := &LoginMsg{}
	copy(msg.CompID[:], p[0:12])
	copy(msg.Password[:], p[12:32])
	return msg, nil
}

func decodeLogout(p []byte) (*LogoutMsg, error) {
	if len(p) < 32 {
		return nil, fmt.Errorf("logout payload too short: %d < 32", len(p))
	}
	msg := &LogoutMsg{}
	copy(msg.Reason[:], p[0:32])
	return msg, nil
}

func decodeNewOrder(p []byte) (*NewOrderMsg, error) {
	if len(p) < 47 {
		return nil, fmt.Errorf("new order payload too short: %d < 47", len(p))
	}
	msg := &NewOrderMsg{}
	copy(msg.InstrumentID[:], p[0:12])
	msg.Side = p[12]
	msg.OrderType = p[13]
	msg.Quantity = binary.BigEndian.Uint32(p[14:18])
	msg.Price = binary.BigEndian.Uint64(p[18:26])
	msg.TimeInForce = p[26]
	copy(msg.ClientOrderID[:], p[27:47])
	return msg, nil
}

func decodeCancelOrder(p []byte) (*CancelOrderMsg, error) {
	if len(p) < 32 {
		return nil, fmt.Errorf("cancel order payload too short: %d < 32", len(p))
	}
	msg := &CancelOrderMsg{}
	copy(msg.OrigClientOrderID[:], p[0:20])
	copy(msg.InstrumentID[:], p[20:32])
	return msg, nil
}

func decodeExecutionReport(p []byte) (*ExecutionReportMsg, error) {
	if len(p) < 63 {
		return nil, fmt.Errorf("execution report payload too short: %d < 63", len(p))
	}
	msg := &ExecutionReportMsg{}
	copy(msg.OrderID[:], p[0:20])
	copy(msg.ExecID[:], p[20:40])
	msg.ExecType = p[40]
	msg.OrdStatus = p[41]
	msg.Side = p[42]
	msg.Quantity = binary.BigEndian.Uint32(p[43:47])
	msg.Price = binary.BigEndian.Uint64(p[47:55])
	msg.LeavesQty = binary.BigEndian.Uint32(p[55:59])
	msg.CumQty = binary.BigEndian.Uint32(p[59:63])
	return msg, nil
}

func decodeReject(p []byte) (*RejectMsg, error) {
	if len(p) < 66 {
		return nil, fmt.Errorf("reject payload too short: %d < 66", len(p))
	}
	msg := &RejectMsg{}
	msg.RefMsgType = binary.BigEndian.Uint16(p[0:2])
	copy(msg.Reason[:], p[2:66])
	return msg, nil
}
