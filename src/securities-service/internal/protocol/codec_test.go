package protocol

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"testing"
)

// padRight writes s into a fixed-size byte array, right-padded with spaces.
func padRight(size int, s string) []byte {
	b := make([]byte, size)
	for i := range b {
		b[i] = ' '
	}
	copy(b, s)
	return b
}

func toArray12(s string) [12]byte {
	var a [12]byte
	p := padRight(12, s)
	copy(a[:], p)
	return a
}

func toArray20(s string) [20]byte {
	var a [20]byte
	p := padRight(20, s)
	copy(a[:], p)
	return a
}

func toArray32(s string) [32]byte {
	var a [32]byte
	p := padRight(32, s)
	copy(a[:], p)
	return a
}

func toArray64(s string) [64]byte {
	var a [64]byte
	p := padRight(64, s)
	copy(a[:], p)
	return a
}

// roundTrip encodes a message and decodes it, returning the header and decoded message.
func roundTrip(t *testing.T, msg Message) (*Header, Message) {
	t.Helper()
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	if err := enc.Encode(msg); err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	dec := NewDecoder(&buf)
	hdr, decoded, err := dec.Decode()
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	return hdr, decoded
}

func TestHeader_RoundTrip(t *testing.T) {
	msg := &HeartbeatMsg{}
	hdr, _ := roundTrip(t, msg)

	if hdr.MsgType != MsgHeartbeat {
		t.Errorf("MsgType = 0x%04X, want 0x%04X", hdr.MsgType, MsgHeartbeat)
	}
	if hdr.Length != HeaderSize {
		t.Errorf("Length = %d, want %d", hdr.Length, HeaderSize)
	}
	if hdr.Sequence != 1 {
		t.Errorf("Sequence = %d, want 1", hdr.Sequence)
	}
}

func TestLoginMsg_RoundTrip(t *testing.T) {
	orig := &LoginMsg{
		CompID:   toArray12("BROKER01"),
		Password: toArray20("s3cret!pass"),
	}

	hdr, decoded := roundTrip(t, orig)

	if hdr.MsgType != MsgLogin {
		t.Errorf("MsgType = 0x%04X, want 0x%04X", hdr.MsgType, MsgLogin)
	}
	if hdr.Length != HeaderSize+32 {
		t.Errorf("Length = %d, want %d", hdr.Length, HeaderSize+32)
	}

	login, ok := decoded.(*LoginMsg)
	if !ok {
		t.Fatalf("decoded type = %T, want *LoginMsg", decoded)
	}
	if login.CompID != orig.CompID {
		t.Errorf("CompID = %q, want %q", login.CompID, orig.CompID)
	}
	if login.Password != orig.Password {
		t.Errorf("Password = %q, want %q", login.Password, orig.Password)
	}
}

func TestNewOrderMsg_RoundTrip(t *testing.T) {
	price := PriceToFixed(850.25) // should be 8502500

	orig := &NewOrderMsg{
		InstrumentID:  toArray12("WHEAT-DEC26"),
		Side:          'B',
		OrderType:     'L',
		Quantity:      1000,
		Price:         price,
		TimeInForce:   '0',
		ClientOrderID: toArray20("ORD-20260425-001"),
	}

	hdr, decoded := roundTrip(t, orig)

	if hdr.MsgType != MsgNewOrder {
		t.Errorf("MsgType = 0x%04X, want 0x%04X", hdr.MsgType, MsgNewOrder)
	}
	if hdr.Length != HeaderSize+47 {
		t.Errorf("Length = %d, want %d", hdr.Length, HeaderSize+47)
	}

	order, ok := decoded.(*NewOrderMsg)
	if !ok {
		t.Fatalf("decoded type = %T, want *NewOrderMsg", decoded)
	}

	if order.InstrumentID != orig.InstrumentID {
		t.Errorf("InstrumentID = %q, want %q", order.InstrumentID, orig.InstrumentID)
	}
	if order.Side != 'B' {
		t.Errorf("Side = %c, want B", order.Side)
	}
	if order.OrderType != 'L' {
		t.Errorf("OrderType = %c, want L", order.OrderType)
	}
	if order.Quantity != 1000 {
		t.Errorf("Quantity = %d, want 1000", order.Quantity)
	}
	if order.Price != 8502500 {
		t.Errorf("Price = %d, want 8502500", order.Price)
	}
	if order.TimeInForce != '0' {
		t.Errorf("TimeInForce = %c, want 0", order.TimeInForce)
	}
	if order.ClientOrderID != orig.ClientOrderID {
		t.Errorf("ClientOrderID = %q, want %q", order.ClientOrderID, orig.ClientOrderID)
	}
}

func TestExecutionReport_RoundTrip(t *testing.T) {
	orig := &ExecutionReportMsg{
		OrderID:   toArray20("EXC-ORD-00042"),
		ExecID:    toArray20("EXEC-00099"),
		ExecType:  '1', // Partial Fill
		OrdStatus: '1', // Partially Filled
		Side:      'S',
		Quantity:  500,
		Price:     PriceToFixed(123.4567), // 1234567
		LeavesQty: 200,
		CumQty:    300,
	}

	hdr, decoded := roundTrip(t, orig)

	if hdr.MsgType != MsgExecutionReport {
		t.Errorf("MsgType = 0x%04X, want 0x%04X", hdr.MsgType, MsgExecutionReport)
	}
	if hdr.Length != HeaderSize+63 {
		t.Errorf("Length = %d, want %d", hdr.Length, HeaderSize+63)
	}

	er, ok := decoded.(*ExecutionReportMsg)
	if !ok {
		t.Fatalf("decoded type = %T, want *ExecutionReportMsg", decoded)
	}

	if er.OrderID != orig.OrderID {
		t.Errorf("OrderID = %q, want %q", er.OrderID, orig.OrderID)
	}
	if er.ExecID != orig.ExecID {
		t.Errorf("ExecID = %q, want %q", er.ExecID, orig.ExecID)
	}
	if er.ExecType != '1' {
		t.Errorf("ExecType = %c, want 1", er.ExecType)
	}
	if er.OrdStatus != '1' {
		t.Errorf("OrdStatus = %c, want 1", er.OrdStatus)
	}
	if er.Side != 'S' {
		t.Errorf("Side = %c, want S", er.Side)
	}
	if er.Quantity != 500 {
		t.Errorf("Quantity = %d, want 500", er.Quantity)
	}
	if er.Price != 1234567 {
		t.Errorf("Price = %d, want 1234567", er.Price)
	}
	if er.LeavesQty != 200 {
		t.Errorf("LeavesQty = %d, want 200", er.LeavesQty)
	}
	if er.CumQty != 300 {
		t.Errorf("CumQty = %d, want 300", er.CumQty)
	}
}

func TestHeartbeat_RoundTrip(t *testing.T) {
	orig := &HeartbeatMsg{}
	hdr, decoded := roundTrip(t, orig)

	if hdr.MsgType != MsgHeartbeat {
		t.Errorf("MsgType = 0x%04X, want 0x%04X", hdr.MsgType, MsgHeartbeat)
	}
	if hdr.Length != HeaderSize {
		t.Errorf("Length = %d, want %d (header only)", hdr.Length, HeaderSize)
	}

	_, ok := decoded.(*HeartbeatMsg)
	if !ok {
		t.Fatalf("decoded type = %T, want *HeartbeatMsg", decoded)
	}
}

func TestCancelOrder_RoundTrip(t *testing.T) {
	orig := &CancelOrderMsg{
		OrigClientOrderID: toArray20("ORD-20260425-001"),
		InstrumentID:      toArray12("WHEAT-DEC26"),
	}

	hdr, decoded := roundTrip(t, orig)

	if hdr.MsgType != MsgCancelOrder {
		t.Errorf("MsgType = 0x%04X, want 0x%04X", hdr.MsgType, MsgCancelOrder)
	}
	if hdr.Length != HeaderSize+32 {
		t.Errorf("Length = %d, want %d", hdr.Length, HeaderSize+32)
	}

	cancel, ok := decoded.(*CancelOrderMsg)
	if !ok {
		t.Fatalf("decoded type = %T, want *CancelOrderMsg", decoded)
	}

	if cancel.OrigClientOrderID != orig.OrigClientOrderID {
		t.Errorf("OrigClientOrderID = %q, want %q", cancel.OrigClientOrderID, orig.OrigClientOrderID)
	}
	if cancel.InstrumentID != orig.InstrumentID {
		t.Errorf("InstrumentID = %q, want %q", cancel.InstrumentID, orig.InstrumentID)
	}
}

func TestReject_RoundTrip(t *testing.T) {
	orig := &RejectMsg{
		RefMsgType: MsgNewOrder,
		Reason:     toArray64("Invalid instrument"),
	}

	hdr, decoded := roundTrip(t, orig)

	if hdr.MsgType != MsgReject {
		t.Errorf("MsgType = 0x%04X, want 0x%04X", hdr.MsgType, MsgReject)
	}
	if hdr.Length != HeaderSize+66 {
		t.Errorf("Length = %d, want %d", hdr.Length, HeaderSize+66)
	}

	rej, ok := decoded.(*RejectMsg)
	if !ok {
		t.Fatalf("decoded type = %T, want *RejectMsg", decoded)
	}

	if rej.RefMsgType != MsgNewOrder {
		t.Errorf("RefMsgType = 0x%04X, want 0x%04X", rej.RefMsgType, MsgNewOrder)
	}
	if rej.Reason != orig.Reason {
		t.Errorf("Reason = %q, want %q", rej.Reason, orig.Reason)
	}
}

func TestDecoder_TruncatedHeader(t *testing.T) {
	// Only 6 bytes — less than the 14-byte header
	buf := bytes.NewReader([]byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x0E})
	dec := NewDecoder(buf)

	_, _, err := dec.Decode()
	if err == nil {
		t.Fatal("expected error for truncated header, got nil")
	}
}

func TestDecoder_UnknownMsgType(t *testing.T) {
	// Build a valid header with unknown message type 0xFF00
	var buf bytes.Buffer
	binary.BigEndian.PutUint16(writeSlice(&buf, 2), 0xFF00)
	binary.BigEndian.PutUint32(writeSlice(&buf, 4), HeaderSize) // no payload
	binary.BigEndian.PutUint64(writeSlice(&buf, 8), 1)

	dec := NewDecoder(&buf)
	_, _, err := dec.Decode()
	if err == nil {
		t.Fatal("expected error for unknown message type, got nil")
	}
}

// writeSlice appends n zero bytes to the buffer and returns the slice for writing.
func writeSlice(buf *bytes.Buffer, n int) []byte {
	start := buf.Len()
	buf.Write(make([]byte, n))
	return buf.Bytes()[start : start+n]
}

func TestPriceFixedPoint(t *testing.T) {
	// 850.25 -> 8502500
	fixed := PriceToFixed(850.25)
	if fixed != 8502500 {
		t.Errorf("PriceToFixed(850.25) = %d, want 8502500", fixed)
	}

	// 8502500 -> 850.25
	price := FixedToPrice(8502500)
	if math.Abs(price-850.25) > 1e-9 {
		t.Errorf("FixedToPrice(8502500) = %f, want 850.25", price)
	}

	// Additional cases
	if PriceToFixed(100.0) != 1000000 {
		t.Errorf("PriceToFixed(100.0) = %d, want 1000000", PriceToFixed(100.0))
	}
	if PriceToFixed(0.0001) != 1 {
		t.Errorf("PriceToFixed(0.0001) = %d, want 1", PriceToFixed(0.0001))
	}

	rt := FixedToPrice(PriceToFixed(999.9999))
	if math.Abs(rt-999.9999) > 1e-4 {
		t.Errorf("round-trip 999.9999 = %f", rt)
	}
}

func TestEncoder_SequenceIncrements(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)

	// Encode 3 heartbeats — sequence should be 1, 2, 3
	for i := 0; i < 3; i++ {
		if err := enc.Encode(&HeartbeatMsg{}); err != nil {
			t.Fatalf("encode %d: %v", i, err)
		}
	}

	dec := NewDecoder(&buf)
	for i := uint64(1); i <= 3; i++ {
		hdr, _, err := dec.Decode()
		if err != nil {
			t.Fatalf("decode %d: %v", i, err)
		}
		if hdr.Sequence != i {
			t.Errorf("message %d: Sequence = %d, want %d", i, hdr.Sequence, i)
		}
	}
}

func TestLogout_RoundTrip(t *testing.T) {
	orig := &LogoutMsg{
		Reason: toArray32("Session timeout"),
	}

	hdr, decoded := roundTrip(t, orig)

	if hdr.MsgType != MsgLogout {
		t.Errorf("MsgType = 0x%04X, want 0x%04X", hdr.MsgType, MsgLogout)
	}

	logout, ok := decoded.(*LogoutMsg)
	if !ok {
		t.Fatalf("decoded type = %T, want *LogoutMsg", decoded)
	}
	if logout.Reason != orig.Reason {
		t.Errorf("Reason = %q, want %q", logout.Reason, orig.Reason)
	}
}

func TestDecoder_TruncatedPayload(t *testing.T) {
	// Valid header claiming 46 bytes total (Login), but only provide header + 5 payload bytes
	var buf bytes.Buffer
	hdr := make([]byte, HeaderSize)
	binary.BigEndian.PutUint16(hdr[0:2], MsgLogin)
	binary.BigEndian.PutUint32(hdr[2:6], HeaderSize+32) // claims 32 bytes payload
	binary.BigEndian.PutUint64(hdr[6:14], 1)
	buf.Write(hdr)
	buf.Write([]byte{0x01, 0x02, 0x03, 0x04, 0x05}) // only 5 of 32 payload bytes

	dec := NewDecoder(&buf)
	_, _, err := dec.Decode()
	if err == nil {
		t.Fatal("expected error for truncated payload, got nil")
	}
}

func TestMultipleMessages_Stream(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)

	// Write a login, a new order, and a heartbeat
	login := &LoginMsg{CompID: toArray12("FIRM-A"), Password: toArray20("pass123")}
	order := &NewOrderMsg{
		InstrumentID:  toArray12("GOLD"),
		Side:          'B',
		OrderType:     'M',
		Quantity:      50,
		Price:         PriceToFixed(1800.00),
		TimeInForce:   '2',
		ClientOrderID: toArray20("CLO-001"),
	}
	hb := &HeartbeatMsg{}

	if err := enc.Encode(login); err != nil {
		t.Fatalf("encode login: %v", err)
	}
	if err := enc.Encode(order); err != nil {
		t.Fatalf("encode order: %v", err)
	}
	if err := enc.Encode(hb); err != nil {
		t.Fatalf("encode heartbeat: %v", err)
	}

	dec := NewDecoder(&buf)

	// Decode login
	h1, m1, err := dec.Decode()
	if err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if h1.Sequence != 1 {
		t.Errorf("login seq = %d, want 1", h1.Sequence)
	}
	if _, ok := m1.(*LoginMsg); !ok {
		t.Errorf("message 1 type = %T, want *LoginMsg", m1)
	}

	// Decode order
	h2, m2, err := dec.Decode()
	if err != nil {
		t.Fatalf("decode order: %v", err)
	}
	if h2.Sequence != 2 {
		t.Errorf("order seq = %d, want 2", h2.Sequence)
	}
	if _, ok := m2.(*NewOrderMsg); !ok {
		t.Errorf("message 2 type = %T, want *NewOrderMsg", m2)
	}

	// Decode heartbeat
	h3, m3, err := dec.Decode()
	if err != nil {
		t.Fatalf("decode heartbeat: %v", err)
	}
	if h3.Sequence != 3 {
		t.Errorf("heartbeat seq = %d, want 3", h3.Sequence)
	}
	if _, ok := m3.(*HeartbeatMsg); !ok {
		t.Errorf("message 3 type = %T, want *HeartbeatMsg", m3)
	}

	// No more messages
	_, _, err = dec.Decode()
	if err != io.EOF && err.Error() != "read header: EOF" {
		t.Errorf("expected EOF after all messages, got: %v", err)
	}
}
