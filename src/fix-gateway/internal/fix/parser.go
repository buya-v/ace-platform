package fix

import (
	"fmt"
	"strconv"
	"strings"
)

// FIX tag constants.
const (
	TagBeginString   = 8
	TagBodyLength    = 9
	TagMsgType       = 35
	TagSenderCompID  = 49
	TagTargetCompID  = 56
	TagMsgSeqNum     = 34
	TagCheckSum      = 10
	TagSendingTime   = 52
	TagText          = 58
	TagEncryptMethod = 98
	TagHeartBtInt    = 108
	TagResetSeqNum   = 141
	TagUsername       = 553
	TagPassword      = 554
	TagTestReqID     = 112

	// Application tags — orders.
	TagClOrdID      = 11
	TagOrigClOrdID  = 41
	TagOrderID      = 37
	TagExecID       = 17
	TagExecType     = 150
	TagOrdStatus    = 39
	TagSymbol       = 55
	TagSecurityID   = 48
	TagSecurityIDSrc = 22
	TagSide         = 54
	TagOrderQty     = 38
	TagOrdType      = 40
	TagPrice        = 44
	TagStopPx       = 99
	TagTimeInForce  = 59
	TagExpireDate   = 432
	TagAccount      = 1
	TagTransactTime = 60
	TagLeavesQty    = 151
	TagCumQty       = 14
	TagAvgPx        = 6
	TagLastPx       = 31
	TagLastQty      = 32
	TagLastMkt      = 30
	TagTrdMatchID   = 880
	TagOrdRejReason = 103
	TagCxlRejReason = 102
	TagCxlRejResponseTo = 434

	// Market data tags.
	TagMDReqID              = 262
	TagSubscriptionReqType  = 263
	TagMarketDepth          = 264
	TagNoMDEntryTypes       = 267
	TagMDEntryType          = 269
	TagNoRelatedSym         = 146
	TagMDUpdateType         = 265
	TagNoMDEntries          = 268
	TagMDEntryPx            = 270
	TagMDEntrySize          = 271
	TagMDEntryDate          = 272
	TagMDEntryTime          = 273
	TagMDEntryPositionNo    = 290
	TagMDUpdateAction       = 279
	TagMDReqRejReason       = 281

	// Session-level message types.
	MsgTypeLogon          = "A"
	MsgTypeLogout         = "5"
	MsgTypeHeartbeat      = "0"
	MsgTypeTestRequest    = "1"
	MsgTypeResendRequest  = "2"
	MsgTypeReject         = "3"
	MsgTypeSequenceReset  = "4"

	// Application-level message types.
	MsgTypeNewOrderSingle       = "D"
	MsgTypeExecutionReport      = "8"
	MsgTypeOrderCancelRequest   = "F"
	MsgTypeOrderCancelReject    = "9"
	MsgTypeCancelReplaceRequest = "G"
	MsgTypeMarketDataRequest    = "V"
	MsgTypeMDSnapshot           = "W"
	MsgTypeMDIncremental        = "X"
	MsgTypeMDRequestReject      = "Y"
	MsgTypeBusinessReject       = "j"

	// SOH delimiter (ASCII 0x01).
	SOH = byte(0x01)

	// BeginString value.
	BeginStringFIX44 = "FIX.4.4"
)

// FIXMessage represents a parsed FIX protocol message.
type FIXMessage struct {
	Fields map[int]string
}

// ParseMessage parses a raw FIX message from wire format.
// Fields are delimited by SOH (0x01), each field is tag=value.
func ParseMessage(data []byte) (*FIXMessage, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty message")
	}

	msg := &FIXMessage{
		Fields: make(map[int]string),
	}

	raw := string(data)
	// Support both SOH and pipe as delimiters (pipe for testing/debugging).
	var delim string
	if strings.Contains(raw, string(SOH)) {
		delim = string(SOH)
	} else {
		delim = "|"
	}

	parts := strings.Split(raw, delim)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eqIdx := strings.IndexByte(part, '=')
		if eqIdx < 1 {
			continue
		}
		tagStr := part[:eqIdx]
		value := part[eqIdx+1:]

		tag, err := strconv.Atoi(tagStr)
		if err != nil {
			return nil, fmt.Errorf("invalid tag %q: %w", tagStr, err)
		}
		msg.Fields[tag] = value
	}

	if len(msg.Fields) == 0 {
		return nil, fmt.Errorf("no valid fields parsed")
	}

	return msg, nil
}

// BuildMessage constructs a raw FIX message with proper header, body length, and checksum.
func BuildMessage(msgType string, senderCompID, targetCompID string, seqNum int, fields map[int]string) []byte {
	// Build body (everything between BeginString+BodyLength and CheckSum).
	var body strings.Builder

	// Header fields (after BeginString and BodyLength).
	writeField(&body, TagMsgType, msgType)
	writeField(&body, TagSenderCompID, senderCompID)
	writeField(&body, TagTargetCompID, targetCompID)
	writeField(&body, TagMsgSeqNum, strconv.Itoa(seqNum))

	// Additional fields sorted is not required by FIX but helps determinism.
	for tag, value := range fields {
		// Skip header/trailer tags that we handle explicitly.
		if tag == TagBeginString || tag == TagBodyLength || tag == TagMsgType ||
			tag == TagSenderCompID || tag == TagTargetCompID || tag == TagMsgSeqNum ||
			tag == TagCheckSum {
			continue
		}
		writeField(&body, tag, value)
	}

	bodyStr := body.String()
	bodyLen := len(bodyStr)

	// Build full message: BeginString + BodyLength + body + CheckSum.
	var msg strings.Builder
	writeField(&msg, TagBeginString, BeginStringFIX44)
	writeField(&msg, TagBodyLength, strconv.Itoa(bodyLen))
	msg.WriteString(bodyStr)

	// Calculate checksum over everything before the checksum field.
	checksum := CalculateChecksum([]byte(msg.String()))
	writeField(&msg, TagCheckSum, checksum)

	return []byte(msg.String())
}

// CalculateChecksum computes the FIX checksum: sum of all bytes modulo 256, formatted as 3-digit string.
func CalculateChecksum(data []byte) string {
	var sum int
	for _, b := range data {
		sum += int(b)
	}
	return fmt.Sprintf("%03d", sum%256)
}

// GetTag returns the string value for a tag, or empty string if not present.
func GetTag(msg *FIXMessage, tag int) string {
	if msg == nil {
		return ""
	}
	return msg.Fields[tag]
}

// GetIntTag returns the integer value for a tag, or 0 if not present or not a valid integer.
func GetIntTag(msg *FIXMessage, tag int) int {
	v := GetTag(msg, tag)
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}

// GetFloatTag returns the float64 value for a tag, or 0.0 if not present or not a valid float.
func GetFloatTag(msg *FIXMessage, tag int) float64 {
	v := GetTag(msg, tag)
	if v == "" {
		return 0.0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0.0
	}
	return f
}

func writeField(b *strings.Builder, tag int, value string) {
	b.WriteString(strconv.Itoa(tag))
	b.WriteByte('=')
	b.WriteString(value)
	b.WriteByte(SOH)
}
