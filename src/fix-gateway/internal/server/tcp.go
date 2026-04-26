package server

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/garudax-platform/fix-gateway/internal/broker"
	"github.com/garudax-platform/fix-gateway/internal/fix"
	"github.com/garudax-platform/fix-gateway/internal/router"
	"github.com/garudax-platform/fix-gateway/internal/session"
)

// connState tracks per-connection metadata.
type connState struct {
	conn         net.Conn
	senderCompID string
	targetCompID string
	tenantID     string
	stopHB       chan struct{} // signals heartbeat goroutine to stop
}

// FIXServer accepts TCP connections from FIX broker clients.
type FIXServer struct {
	logger         *slog.Logger
	listener       net.Listener
	sessionManager *session.SessionManager
	brokerStore    broker.BrokerStore
	orderRouter    *router.OrderRouter
	targetCompID   string

	mu          sync.RWMutex
	connections map[net.Conn]*connState
	closed      bool
}

// NewFIXServer creates a new FIXServer.
func NewFIXServer(
	logger *slog.Logger,
	sessionMgr *session.SessionManager,
	brokerStore broker.BrokerStore,
	orderRouter *router.OrderRouter,
	targetCompID string,
) *FIXServer {
	return &FIXServer{
		logger:         logger,
		sessionManager: sessionMgr,
		brokerStore:    brokerStore,
		orderRouter:    orderRouter,
		targetCompID:   targetCompID,
		connections:    make(map[net.Conn]*connState),
	}
}

// Start begins listening for FIX TCP connections on the given address.
func (s *FIXServer) Start(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("tcp listen %s: %w", addr, err)
	}
	s.listener = ln
	s.logger.Info("FIX TCP server listening", slog.String("addr", addr))

	go s.acceptLoop()
	return nil
}

func (s *FIXServer) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.RLock()
			closed := s.closed
			s.mu.RUnlock()
			if closed {
				return
			}
			s.logger.Error("accept error", slog.String("error", err.Error()))
			continue
		}
		s.logger.Info("new FIX connection", slog.String("remote", conn.RemoteAddr().String()))
		go s.handleConnection(conn)
	}
}

// Stop closes the listener and all active connections.
func (s *FIXServer) Stop() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()

	var firstErr error
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			firstErr = err
		}
	}

	s.mu.RLock()
	conns := make([]*connState, 0, len(s.connections))
	for _, cs := range s.connections {
		conns = append(conns, cs)
	}
	s.mu.RUnlock()

	for _, cs := range conns {
		close(cs.stopHB)
		cs.conn.Close()
	}

	return firstErr
}

// handleConnection reads FIX messages from a single TCP connection.
func (s *FIXServer) handleConnection(conn net.Conn) {
	cs := &connState{
		conn:   conn,
		stopHB: make(chan struct{}),
	}

	s.mu.Lock()
	s.connections[conn] = cs
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.connections, conn)
		s.mu.Unlock()
		conn.Close()
	}()

	buf := make([]byte, 8192)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				s.logger.Error("read error",
					slog.String("remote", conn.RemoteAddr().String()),
					slog.String("error", err.Error()),
				)
			}
			// Clean up session if established.
			if cs.senderCompID != "" {
				_ = s.sessionManager.ProcessLogout(cs.senderCompID, cs.targetCompID, cs.tenantID)
			}
			return
		}

		msg, err := fix.ParseMessage(buf[:n])
		if err != nil {
			s.logger.Warn("parse error",
				slog.String("remote", conn.RemoteAddr().String()),
				slog.String("error", err.Error()),
			)
			continue
		}

		s.dispatch(cs, msg)
	}
}

// dispatch routes a parsed FIX message by MsgType.
func (s *FIXServer) dispatch(cs *connState, msg *fix.FIXMessage) {
	msgType := fix.GetTag(msg, fix.TagMsgType)

	switch msgType {
	case fix.MsgTypeLogon:
		s.handleLogon(cs, msg)
	case fix.MsgTypeNewOrderSingle:
		s.handleNewOrderSingle(cs, msg)
	case fix.MsgTypeOrderCancelRequest:
		s.handleOrderCancelRequest(cs, msg)
	case fix.MsgTypeHeartbeat:
		s.handleHeartbeat(cs, msg)
	case fix.MsgTypeTestRequest:
		s.handleTestRequest(cs, msg)
	case fix.MsgTypeLogout:
		s.handleLogout(cs, msg)
	default:
		s.logger.Warn("unsupported MsgType",
			slog.String("msg_type", msgType),
			slog.String("remote", cs.conn.RemoteAddr().String()),
		)
	}
}

// handleLogon validates CompID via broker store, creates a session, and sends Logon ack.
func (s *FIXServer) handleLogon(cs *connState, msg *fix.FIXMessage) {
	senderCompID := fix.GetTag(msg, fix.TagSenderCompID)
	targetCompID := fix.GetTag(msg, fix.TagTargetCompID)

	// Validate the sender's CompID against the broker registry.
	b, err := s.brokerStore.GetByCompID(senderCompID)
	if err != nil {
		s.logger.Warn("logon rejected: unknown CompID",
			slog.String("sender_comp_id", senderCompID),
		)
		s.sendLogout(cs, "Unknown CompID: "+senderCompID)
		return
	}

	if b.Status != "ACTIVE" {
		s.logger.Warn("logon rejected: broker not active",
			slog.String("broker_id", b.ID),
			slog.String("status", b.Status),
		)
		s.sendLogout(cs, "Broker not active")
		return
	}

	// Determine heartbeat interval (default 30s).
	hbInterval := fix.GetIntTag(msg, fix.TagHeartBtInt)
	if hbInterval <= 0 {
		hbInterval = 30
	}

	// Create session.
	sess := s.sessionManager.CreateSession(senderCompID, targetCompID, b.TenantID, hbInterval)
	_ = s.sessionManager.ProcessLogon(senderCompID, targetCompID, b.TenantID)

	cs.senderCompID = senderCompID
	cs.targetCompID = targetCompID
	cs.tenantID = b.TenantID

	s.logger.Info("logon accepted",
		slog.String("sender_comp_id", senderCompID),
		slog.String("tenant_id", b.TenantID),
		slog.String("broker_id", b.ID),
	)

	// Send Logon acknowledgement (swap sender/target for response).
	seqNum, _ := s.sessionManager.IncrementOutSeq(senderCompID, targetCompID, b.TenantID)
	ackFields := map[int]string{
		fix.TagEncryptMethod: "0",
		fix.TagHeartBtInt:    strconv.Itoa(sess.HeartbeatInterval),
	}
	ack := fix.BuildMessage(fix.MsgTypeLogon, s.targetCompID, senderCompID, int(seqNum), ackFields)
	_, _ = cs.conn.Write(ack)

	// Start heartbeat monitor.
	go s.heartbeatLoop(cs)
}

// handleNewOrderSingle maps the FIX order, routes it via OrderRouter, and responds with ExecutionReport.
func (s *FIXServer) handleNewOrderSingle(cs *connState, msg *fix.FIXMessage) {
	if cs.tenantID == "" {
		s.logger.Warn("order rejected: not logged in", slog.String("remote", cs.conn.RemoteAddr().String()))
		return
	}

	order, err := fix.MapNewOrderSingle(msg)
	if err != nil {
		s.logger.Warn("order mapping error",
			slog.String("error", err.Error()),
			slog.String("remote", cs.conn.RemoteAddr().String()),
		)
		// Send Reject.
		s.sendBusinessReject(cs, fix.GetTag(msg, fix.TagClOrdID), err.Error())
		return
	}

	// Build order payload for securities service.
	orderPayload := map[string]interface{}{
		"instrument_id":  order.InstrumentID,
		"side":           order.Side,
		"order_type":     order.OrderType,
		"quantity":       order.Quantity,
		"price":          order.Price,
		"stop_price":     order.StopPrice,
		"time_in_force":  order.TimeInForce,
		"client_order_id": order.ClientOrderID,
		"account":        order.Account,
		"is_short_sell":  order.IsShortSell,
	}

	resp, err := s.orderRouter.SubmitOrder(orderPayload, cs.tenantID)
	if err != nil {
		s.logger.Error("order routing error",
			slog.String("error", err.Error()),
			slog.String("client_order_id", order.ClientOrderID),
		)
		// Send execution report with rejection.
		s.sendExecutionReport(cs, order.ClientOrderID, "8", "8", order.ClientOrderID, 0)
		return
	}

	// Extract order ID from response.
	orderID, _ := resp["order_id"].(string)
	if orderID == "" {
		orderID = order.ClientOrderID
	}

	s.logger.Info("order submitted",
		slog.String("client_order_id", order.ClientOrderID),
		slog.String("order_id", orderID),
		slog.String("tenant_id", cs.tenantID),
	)

	// Send ExecutionReport (New).
	s.sendExecutionReport(cs, orderID, "0", "0", order.ClientOrderID, order.Quantity)
}

// handleOrderCancelRequest routes a cancel request.
func (s *FIXServer) handleOrderCancelRequest(cs *connState, msg *fix.FIXMessage) {
	if cs.tenantID == "" {
		return
	}

	orderID := fix.GetTag(msg, fix.TagOrderID)
	origClOrdID := fix.GetTag(msg, fix.TagOrigClOrdID)

	cancelID := orderID
	if cancelID == "" {
		cancelID = origClOrdID
	}

	s.logger.Info("cancel request",
		slog.String("order_id", cancelID),
		slog.String("tenant_id", cs.tenantID),
	)

	err := s.orderRouter.CancelOrder(cancelID, cs.tenantID)
	if err != nil {
		s.logger.Error("cancel routing error",
			slog.String("error", err.Error()),
			slog.String("order_id", cancelID),
		)
		// Send OrderCancelReject.
		s.sendCancelReject(cs, cancelID, origClOrdID, err.Error())
		return
	}

	// Send ExecutionReport (Canceled).
	s.sendExecutionReport(cs, cancelID, "4", "4", origClOrdID, 0)
}

// handleHeartbeat updates the session's LastRecvTime.
func (s *FIXServer) handleHeartbeat(cs *connState, _ *fix.FIXMessage) {
	if cs.senderCompID != "" {
		s.sessionManager.UpdateLastRecv(cs.senderCompID, cs.targetCompID, cs.tenantID)
	}
}

// handleTestRequest responds with a Heartbeat containing the TestReqID.
func (s *FIXServer) handleTestRequest(cs *connState, msg *fix.FIXMessage) {
	if cs.senderCompID == "" {
		return
	}
	testReqID := fix.GetTag(msg, fix.TagTestReqID)
	seqNum, _ := s.sessionManager.IncrementOutSeq(cs.senderCompID, cs.targetCompID, cs.tenantID)
	fields := map[int]string{}
	if testReqID != "" {
		fields[fix.TagTestReqID] = testReqID
	}
	hb := fix.BuildMessage(fix.MsgTypeHeartbeat, s.targetCompID, cs.senderCompID, int(seqNum), fields)
	_, _ = cs.conn.Write(hb)

	s.sessionManager.UpdateLastRecv(cs.senderCompID, cs.targetCompID, cs.tenantID)
}

// handleLogout closes the session and the connection.
func (s *FIXServer) handleLogout(cs *connState, _ *fix.FIXMessage) {
	if cs.senderCompID != "" {
		_ = s.sessionManager.ProcessLogout(cs.senderCompID, cs.targetCompID, cs.tenantID)
		s.logger.Info("logout",
			slog.String("sender_comp_id", cs.senderCompID),
			slog.String("tenant_id", cs.tenantID),
		)
	}

	// Send Logout ack.
	if cs.senderCompID != "" {
		seqNum, _ := s.sessionManager.IncrementOutSeq(cs.senderCompID, cs.targetCompID, cs.tenantID)
		logout := fix.BuildMessage(fix.MsgTypeLogout, s.targetCompID, cs.senderCompID, int(seqNum), nil)
		_, _ = cs.conn.Write(logout)
	}

	close(cs.stopHB)
	cs.conn.Close()
}

// ---------- heartbeat monitor ----------

// heartbeatLoop sends periodic Heartbeat messages and detects inactivity.
func (s *FIXServer) heartbeatLoop(cs *connState) {
	sess := s.sessionManager.GetSession(cs.senderCompID, cs.targetCompID, cs.tenantID)
	if sess == nil {
		return
	}

	interval := time.Duration(sess.HeartbeatInterval) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	testReqSent := false
	testReqDeadline := time.Time{}

	for {
		select {
		case <-cs.stopHB:
			return
		case <-ticker.C:
			// Check inactivity.
			current := s.sessionManager.GetSession(cs.senderCompID, cs.targetCompID, cs.tenantID)
			if current == nil {
				return
			}

			sinceLastRecv := time.Since(current.LastRecvTime)

			if testReqSent {
				// We already sent TestRequest; check if response deadline passed.
				if time.Now().After(testReqDeadline) {
					s.logger.Warn("heartbeat timeout, disconnecting",
						slog.String("sender_comp_id", cs.senderCompID),
						slog.Duration("silence", sinceLastRecv),
					)
					_ = s.sessionManager.ProcessLogout(cs.senderCompID, cs.targetCompID, cs.tenantID)
					cs.conn.Close()
					return
				}
				continue
			}

			if sinceLastRecv > 2*interval {
				// No message in 2x heartbeat interval — send TestRequest.
				s.logger.Info("sending TestRequest",
					slog.String("sender_comp_id", cs.senderCompID),
					slog.Duration("silence", sinceLastRecv),
				)
				seqNum, _ := s.sessionManager.IncrementOutSeq(cs.senderCompID, cs.targetCompID, cs.tenantID)
				testID := fmt.Sprintf("TEST-%d", time.Now().UnixMilli())
				fields := map[int]string{
					fix.TagTestReqID: testID,
				}
				tr := fix.BuildMessage(fix.MsgTypeTestRequest, s.targetCompID, cs.senderCompID, int(seqNum), fields)
				_, _ = cs.conn.Write(tr)

				testReqSent = true
				testReqDeadline = time.Now().Add(interval) // 30 more seconds
			} else {
				// Send heartbeat.
				seqNum, _ := s.sessionManager.IncrementOutSeq(cs.senderCompID, cs.targetCompID, cs.tenantID)
				hb := fix.BuildMessage(fix.MsgTypeHeartbeat, s.targetCompID, cs.senderCompID, int(seqNum), nil)
				_, _ = cs.conn.Write(hb)
			}

			// Reset test-req state if we received something since last check.
			if testReqSent && sinceLastRecv < interval {
				testReqSent = false
				testReqDeadline = time.Time{}
			}
		}
	}
}

// ---------- response helpers ----------

func (s *FIXServer) sendExecutionReport(cs *connState, orderID, execType, ordStatus, clOrdID string, leavesQty int) {
	seqNum, _ := s.sessionManager.IncrementOutSeq(cs.senderCompID, cs.targetCompID, cs.tenantID)
	fields := map[int]string{
		fix.TagOrderID:      orderID,
		fix.TagExecID:       fmt.Sprintf("EXEC-%d", time.Now().UnixNano()),
		fix.TagExecType:     execType,
		fix.TagOrdStatus:    ordStatus,
		fix.TagClOrdID:      clOrdID,
		fix.TagLeavesQty:    strconv.Itoa(leavesQty),
		fix.TagCumQty:       "0",
		fix.TagAvgPx:        "0.0000",
		fix.TagTransactTime: time.Now().UTC().Format("20060102-15:04:05.000"),
	}
	er := fix.BuildMessage(fix.MsgTypeExecutionReport, s.targetCompID, cs.senderCompID, int(seqNum), fields)
	_, _ = cs.conn.Write(er)
}

func (s *FIXServer) sendBusinessReject(cs *connState, refID, reason string) {
	seqNum, _ := s.sessionManager.IncrementOutSeq(cs.senderCompID, cs.targetCompID, cs.tenantID)
	fields := map[int]string{
		fix.TagText: reason,
	}
	if refID != "" {
		fields[fix.TagClOrdID] = refID
	}
	rej := fix.BuildMessage(fix.MsgTypeBusinessReject, s.targetCompID, cs.senderCompID, int(seqNum), fields)
	_, _ = cs.conn.Write(rej)
}

func (s *FIXServer) sendCancelReject(cs *connState, orderID, origClOrdID, reason string) {
	seqNum, _ := s.sessionManager.IncrementOutSeq(cs.senderCompID, cs.targetCompID, cs.tenantID)
	fields := map[int]string{
		fix.TagOrderID:          orderID,
		fix.TagClOrdID:          origClOrdID,
		fix.TagOrdStatus:        "8", // Rejected
		fix.TagCxlRejResponseTo: "1", // Cancel request
		fix.TagCxlRejReason:     "1", // Unknown order
		fix.TagText:             reason,
	}
	rej := fix.BuildMessage(fix.MsgTypeOrderCancelReject, s.targetCompID, cs.senderCompID, int(seqNum), fields)
	_, _ = cs.conn.Write(rej)
}

func (s *FIXServer) sendLogout(cs *connState, reason string) {
	fields := map[int]string{
		fix.TagText: reason,
	}
	// When rejecting a logon, we may not have a session yet — use seq 1.
	target := cs.senderCompID
	if target == "" {
		target = "UNKNOWN"
	}
	logout := fix.BuildMessage(fix.MsgTypeLogout, s.targetCompID, target, 1, fields)
	_, _ = cs.conn.Write(logout)
	cs.conn.Close()
}
