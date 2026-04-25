package fix

import (
	"strings"
	"testing"
)

// buildSOHMessage constructs a raw FIX message using real SOH delimiters.
func buildSOHMessage(fields []string) []byte {
	return []byte(strings.Join(fields, string(SOH)) + string(SOH))
}

// TestParseMessage_Valid builds a minimal FIX message with SOH delimiters and verifies tags.
func TestParseMessage_Valid(t *testing.T) {
	raw := buildSOHMessage([]string{
		"8=FIX.4.4",
		"9=50",
		"35=0",
		"49=SENDER",
		"56=TARGET",
		"34=1",
		"10=123",
	})

	msg, err := ParseMessage(raw)
	if err != nil {
		t.Fatalf("ParseMessage returned unexpected error: %v", err)
	}

	cases := []struct {
		tag      int
		expected string
	}{
		{TagBeginString, "FIX.4.4"},
		{TagBodyLength, "50"},
		{TagMsgType, MsgTypeHeartbeat},
		{TagSenderCompID, "SENDER"},
		{TagTargetCompID, "TARGET"},
		{TagMsgSeqNum, "1"},
		{TagCheckSum, "123"},
	}

	for _, c := range cases {
		got := GetTag(msg, c.tag)
		if got != c.expected {
			t.Errorf("tag %d: got %q, want %q", c.tag, got, c.expected)
		}
	}
}

// TestParseMessage_Logon parses a Logon message (35=A) and verifies CompID fields.
func TestParseMessage_Logon(t *testing.T) {
	raw := buildSOHMessage([]string{
		"8=FIX.4.4",
		"9=80",
		"35=A",
		"49=BROKER001",
		"56=EXCHANGE",
		"34=1",
		"98=0",
		"108=30",
		"10=100",
	})

	msg, err := ParseMessage(raw)
	if err != nil {
		t.Fatalf("ParseMessage error: %v", err)
	}

	if got := GetTag(msg, TagMsgType); got != MsgTypeLogon {
		t.Errorf("MsgType: got %q, want %q", got, MsgTypeLogon)
	}
	if got := GetTag(msg, TagSenderCompID); got != "BROKER001" {
		t.Errorf("SenderCompID: got %q, want %q", got, "BROKER001")
	}
	if got := GetTag(msg, TagTargetCompID); got != "EXCHANGE" {
		t.Errorf("TargetCompID: got %q, want %q", got, "EXCHANGE")
	}
	if got := GetIntTag(msg, TagEncryptMethod); got != 0 {
		t.Errorf("EncryptMethod: got %d, want 0", got)
	}
	if got := GetIntTag(msg, TagHeartBtInt); got != 30 {
		t.Errorf("HeartBtInt: got %d, want 30", got)
	}
}

// TestParseMessage_NewOrderSingle parses a NewOrderSingle (35=D) and verifies order fields.
func TestParseMessage_NewOrderSingle(t *testing.T) {
	raw := buildSOHMessage([]string{
		"8=FIX.4.4",
		"9=120",
		"35=D",
		"49=BROKER001",
		"56=EXCHANGE",
		"34=5",
		"11=ORD-001",
		"55=AAPL",
		"54=1",
		"40=2",
		"38=100",
		"44=150.25",
		"59=0",
		"60=20260101-09:30:00.000",
		"10=200",
	})

	msg, err := ParseMessage(raw)
	if err != nil {
		t.Fatalf("ParseMessage error: %v", err)
	}

	if got := GetTag(msg, TagMsgType); got != MsgTypeNewOrderSingle {
		t.Errorf("MsgType: got %q, want %q", got, MsgTypeNewOrderSingle)
	}
	if got := GetTag(msg, TagSymbol); got != "AAPL" {
		t.Errorf("Symbol: got %q, want %q", got, "AAPL")
	}
	if got := GetTag(msg, TagSide); got != "1" {
		t.Errorf("Side: got %q, want 1", got)
	}
	if got := GetTag(msg, TagOrdType); got != "2" {
		t.Errorf("OrdType: got %q, want 2", got)
	}
	if got := GetIntTag(msg, TagOrderQty); got != 100 {
		t.Errorf("OrderQty: got %d, want 100", got)
	}
	if got := GetFloatTag(msg, TagPrice); got != 150.25 {
		t.Errorf("Price: got %f, want 150.25", got)
	}
	if got := GetTag(msg, TagClOrdID); got != "ORD-001" {
		t.Errorf("ClOrdID: got %q, want ORD-001", got)
	}
}

// TestParseMessage_EmptyInput verifies that an empty byte slice returns an error.
func TestParseMessage_EmptyInput(t *testing.T) {
	_, err := ParseMessage([]byte{})
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
}

// TestParseMessage_MalformedTag verifies that a non-numeric tag returns an error.
func TestParseMessage_MalformedTag(t *testing.T) {
	// "XYZ=value" — tag is not a number, should cause Atoi failure
	raw := []byte("XYZ=value\x01")
	_, err := ParseMessage(raw)
	if err == nil {
		t.Fatal("expected error for non-numeric tag, got nil")
	}
}

// TestParseMessage_PipeDelimiter verifies that pipe-delimited messages are also parsed.
func TestParseMessage_PipeDelimiter(t *testing.T) {
	raw := []byte("8=FIX.4.4|35=0|49=SENDER|56=TARGET|34=1|10=100|")

	msg, err := ParseMessage(raw)
	if err != nil {
		t.Fatalf("ParseMessage error: %v", err)
	}
	if got := GetTag(msg, TagBeginString); got != BeginStringFIX44 {
		t.Errorf("BeginString: got %q, want %q", got, BeginStringFIX44)
	}
}

// TestBuildMessage verifies that BuildMessage produces messages with BeginString(8),
// BodyLength(9), and CheckSum(10).
func TestBuildMessage(t *testing.T) {
	fields := map[int]string{
		TagEncryptMethod: "0",
		TagHeartBtInt:    "30",
	}

	raw := BuildMessage(MsgTypeLogon, "BROKER001", "EXCHANGE", 1, fields)
	if len(raw) == 0 {
		t.Fatal("BuildMessage returned empty result")
	}

	msg, err := ParseMessage(raw)
	if err != nil {
		t.Fatalf("ParseMessage on built message error: %v", err)
	}

	if got := GetTag(msg, TagBeginString); got != BeginStringFIX44 {
		t.Errorf("BeginString: got %q, want %q", got, BeginStringFIX44)
	}
	if got := GetTag(msg, TagBodyLength); got == "" {
		t.Error("BodyLength(9) missing from built message")
	}
	if got := GetTag(msg, TagCheckSum); got == "" {
		t.Error("CheckSum(10) missing from built message")
	}
	if got := GetTag(msg, TagMsgType); got != MsgTypeLogon {
		t.Errorf("MsgType: got %q, want %q", got, MsgTypeLogon)
	}
}

// TestBuildMessage_RoundTrip builds a message then parses it and verifies all tags match.
func TestBuildMessage_RoundTrip(t *testing.T) {
	inputFields := map[int]string{
		TagEncryptMethod: "0",
		TagHeartBtInt:    "30",
		TagText:          "hello fix",
	}

	raw := BuildMessage(MsgTypeLogon, "SENDER", "TARGET", 42, inputFields)
	msg, err := ParseMessage(raw)
	if err != nil {
		t.Fatalf("ParseMessage on round-trip error: %v", err)
	}

	if got := GetTag(msg, TagBeginString); got != BeginStringFIX44 {
		t.Errorf("BeginString: got %q, want %q", got, BeginStringFIX44)
	}
	if got := GetTag(msg, TagMsgType); got != MsgTypeLogon {
		t.Errorf("MsgType: got %q, want %q", got, MsgTypeLogon)
	}
	if got := GetTag(msg, TagSenderCompID); got != "SENDER" {
		t.Errorf("SenderCompID: got %q, want SENDER", got)
	}
	if got := GetTag(msg, TagTargetCompID); got != "TARGET" {
		t.Errorf("TargetCompID: got %q, want TARGET", got)
	}
	if got := GetIntTag(msg, TagMsgSeqNum); got != 42 {
		t.Errorf("MsgSeqNum: got %d, want 42", got)
	}
	if got := GetTag(msg, TagText); got != "hello fix" {
		t.Errorf("Text: got %q, want %q", got, "hello fix")
	}
	// CheckSum must be exactly 3 digits.
	cs := GetTag(msg, TagCheckSum)
	if len(cs) != 3 {
		t.Errorf("CheckSum length: got %d, want 3 (value=%q)", len(cs), cs)
	}
}

// TestCalculateChecksum verifies that CalculateChecksum produces a known result
// for a deterministic input.
func TestCalculateChecksum(t *testing.T) {
	// "8=FIX.4.4\x01" — known input, compute expected sum manually in test.
	input := []byte("8=FIX.4.4\x01")
	var sum int
	for _, b := range input {
		sum += int(b)
	}
	expected := sum % 256

	got := CalculateChecksum(input)
	// CalculateChecksum returns a 3-digit decimal string.
	var gotInt int
	if _, err := strings.NewReader(got).Read([]byte{}); err == nil {
		// Parse the returned string.
		for _, c := range got {
			gotInt = gotInt*10 + int(c-'0')
		}
	}
	if gotInt != expected {
		t.Errorf("CalculateChecksum: got %q (%d), want %03d", got, gotInt, expected)
	}
	if len(got) != 3 {
		t.Errorf("CalculateChecksum length: got %d, want 3", len(got))
	}
}

// TestCalculateChecksum_KnownValue uses a well-known checksum value.
func TestCalculateChecksum_KnownValue(t *testing.T) {
	// Empty input: sum=0, checksum="000"
	got := CalculateChecksum([]byte{})
	if got != "000" {
		t.Errorf("checksum of empty: got %q, want \"000\"", got)
	}

	// Single byte 0x01: sum=1, checksum="001"
	got = CalculateChecksum([]byte{0x01})
	if got != "001" {
		t.Errorf("checksum of 0x01: got %q, want \"001\"", got)
	}

	// 256 bytes each 0x01: sum=256, 256%256=0, checksum="000"
	data := make([]byte, 256)
	for i := range data {
		data[i] = 0x01
	}
	got = CalculateChecksum(data)
	if got != "000" {
		t.Errorf("checksum of 256x0x01: got %q, want \"000\"", got)
	}
}

// TestGetTag_Helpers tests GetIntTag and GetFloatTag for valid and invalid values.
func TestGetTag_Helpers(t *testing.T) {
	msg := &FIXMessage{
		Fields: map[int]string{
			TagOrderQty:  "500",
			TagPrice:     "123.456",
			TagBodyLength: "notanumber",
			TagText:      "hello",
		},
	}

	// GetIntTag — valid.
	if got := GetIntTag(msg, TagOrderQty); got != 500 {
		t.Errorf("GetIntTag valid: got %d, want 500", got)
	}

	// GetIntTag — invalid (non-numeric string).
	if got := GetIntTag(msg, TagBodyLength); got != 0 {
		t.Errorf("GetIntTag invalid: got %d, want 0", got)
	}

	// GetIntTag — missing tag.
	if got := GetIntTag(msg, TagStopPx); got != 0 {
		t.Errorf("GetIntTag missing: got %d, want 0", got)
	}

	// GetFloatTag — valid.
	if got := GetFloatTag(msg, TagPrice); got != 123.456 {
		t.Errorf("GetFloatTag valid: got %f, want 123.456", got)
	}

	// GetFloatTag — invalid (non-numeric string).
	if got := GetFloatTag(msg, TagText); got != 0.0 {
		t.Errorf("GetFloatTag invalid: got %f, want 0.0", got)
	}

	// GetFloatTag — missing tag.
	if got := GetFloatTag(msg, TagStopPx); got != 0.0 {
		t.Errorf("GetFloatTag missing: got %f, want 0.0", got)
	}

	// GetTag — nil message.
	if got := GetTag(nil, TagPrice); got != "" {
		t.Errorf("GetTag nil msg: got %q, want \"\"", got)
	}

	// GetIntTag — nil message.
	if got := GetIntTag(nil, TagPrice); got != 0 {
		t.Errorf("GetIntTag nil msg: got %d, want 0", got)
	}

	// GetFloatTag — nil message.
	if got := GetFloatTag(nil, TagPrice); got != 0.0 {
		t.Errorf("GetFloatTag nil msg: got %f, want 0.0", got)
	}
}

// TestParseMessage_OnlyInvalidFields verifies that a message with no parseable fields returns error.
func TestParseMessage_OnlyInvalidFields(t *testing.T) {
	// All fields missing "=" sign — all are skipped, leaving 0 valid fields.
	raw := []byte("notafield\x01alsowrong\x01")
	_, err := ParseMessage(raw)
	if err == nil {
		t.Fatal("expected error for message with no valid fields, got nil")
	}
}

// TestBuildMessage_SkipsHeaderTrailerTags verifies that header/trailer tags in the
// extra fields map are not duplicated in the output.
func TestBuildMessage_SkipsHeaderTrailerTags(t *testing.T) {
	// Pass BeginString and CheckSum explicitly — they should be handled by BuildMessage
	// and not cause duplicate tags.
	fields := map[int]string{
		TagBeginString: "FIX.4.2",   // should be ignored
		TagCheckSum:    "999",        // should be ignored
		TagText:        "uniquetext",
	}

	raw := BuildMessage(MsgTypeHeartbeat, "A", "B", 1, fields)
	msg, err := ParseMessage(raw)
	if err != nil {
		t.Fatalf("ParseMessage error: %v", err)
	}

	// BeginString should be platform default, not overridden.
	if got := GetTag(msg, TagBeginString); got != BeginStringFIX44 {
		t.Errorf("BeginString: got %q, want %q", got, BeginStringFIX44)
	}
	// Text should be present since it is a non-header tag.
	if got := GetTag(msg, TagText); got != "uniquetext" {
		t.Errorf("Text: got %q, want uniquetext", got)
	}
}
