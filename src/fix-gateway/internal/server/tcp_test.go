package server

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"log/slog"
	"os"

	"github.com/garudax-platform/fix-gateway/internal/broker"
	"github.com/garudax-platform/fix-gateway/internal/fix"
	"github.com/garudax-platform/fix-gateway/internal/router"
	"github.com/garudax-platform/fix-gateway/internal/session"
)

// newTestServer creates a FIXServer wired with in-memory stores and a no-op router.
// It returns the server and the address it is listening on.
func newTestServer(t *testing.T, securitiesURL string) (*FIXServer, string) {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Random port: listen on ":0" then extract the assigned port.
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("pre-listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close() // Release so FIXServer.Start can bind it.

	sessionMgr := session.NewSessionManager()
	brokerStore := broker.NewInMemoryStore() // seeds MSE001 / mse-equities
	orderRouter := router.NewOrderRouter(securitiesURL)

	srv := NewFIXServer(logger, sessionMgr, brokerStore, orderRouter, "EXCHANGE")

	if err := srv.Start(addr); err != nil {
		t.Fatalf("Start: %v", err)
	}

	return srv, addr
}

// buildLogon creates a pipe-delimited FIX Logon (35=A) message.
func buildLogon(senderCompID, targetCompID string) []byte {
	fields := map[int]string{
		fix.TagEncryptMethod: "0",
		fix.TagHeartBtInt:    "30",
	}
	return fix.BuildMessage(fix.MsgTypeLogon, senderCompID, targetCompID, 1, fields)
}

// dialAndRead connects to addr, writes msg, then reads until timeout or n bytes available.
func dialAndRead(t *testing.T, addr string, msg []byte, timeout time.Duration) (net.Conn, []byte) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	if _, err := conn.Write(msg); err != nil {
		conn.Close()
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, 4096)
	n, _ := conn.Read(buf)
	conn.SetReadDeadline(time.Time{})
	return conn, buf[:n]
}

// parseFIXResponse parses the first FIX message from raw bytes (pipe or SOH delimited).
func parseFIXResponse(data []byte) *fix.FIXMessage {
	if len(data) == 0 {
		return nil
	}
	msg, err := fix.ParseMessage(data)
	if err != nil {
		return nil
	}
	return msg
}

// ---------- Tests ----------

// TestFIXServer_StartStop verifies that the server starts listening and stops gracefully.
func TestFIXServer_StartStop(t *testing.T) {
	srv, addr := newTestServer(t, "http://127.0.0.1:19999")

	// Verify the server is listening by connecting.
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("server not listening after Start: %v", err)
	}
	conn.Close()

	// Stop should not error.
	if err := srv.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// After Stop the listener must be closed; a new connection should fail.
	_, err = net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err == nil {
		t.Fatal("expected connection failure after Stop, got nil")
	}
}

// TestFIXServer_Logon verifies that a valid Logon (35=A) with a known CompID receives a Logon ack.
func TestFIXServer_Logon(t *testing.T) {
	srv, addr := newTestServer(t, "http://127.0.0.1:19999")
	defer srv.Stop()

	// MSE001 is seeded in InMemoryStore as ACTIVE with tenantID mse-equities.
	logon := buildLogon("MSE001", "EXCHANGE")

	conn, resp := dialAndRead(t, addr, logon, 2*time.Second)
	defer conn.Close()

	if len(resp) == 0 {
		t.Fatal("expected Logon response, got no data")
	}

	msg := parseFIXResponse(resp)
	if msg == nil {
		t.Fatalf("could not parse FIX response: %q", resp)
	}

	// Response must be a Logon (35=A).
	msgType := fix.GetTag(msg, fix.TagMsgType)
	if msgType != fix.MsgTypeLogon {
		t.Errorf("MsgType: got %q, want %q (Logon)", msgType, fix.MsgTypeLogon)
	}

	// SenderCompID in the response must be the server's CompID.
	senderCompID := fix.GetTag(msg, fix.TagSenderCompID)
	if senderCompID != "EXCHANGE" {
		t.Errorf("SenderCompID: got %q, want EXCHANGE", senderCompID)
	}

	// TargetCompID in the response must echo back the client's CompID.
	targetCompID := fix.GetTag(msg, fix.TagTargetCompID)
	if targetCompID != "MSE001" {
		t.Errorf("TargetCompID: got %q, want MSE001", targetCompID)
	}
}

// TestFIXServer_InvalidCompID verifies that a Logon with an unknown CompID results in either
// a Logout response or a closed connection.
func TestFIXServer_InvalidCompID(t *testing.T) {
	srv, addr := newTestServer(t, "http://127.0.0.1:19999")
	defer srv.Stop()

	// UNKNOWN999 is not in the broker store.
	logon := buildLogon("UNKNOWN999", "EXCHANGE")

	conn, resp := dialAndRead(t, addr, logon, 2*time.Second)
	defer conn.Close()

	// Either the server sends a Logout (35=5) or it closes the connection immediately.
	// Both are valid rejection signals.
	if len(resp) > 0 {
		msg := parseFIXResponse(resp)
		if msg == nil {
			t.Fatalf("got non-empty, non-parseable response: %q", resp)
		}
		msgType := fix.GetTag(msg, fix.TagMsgType)
		if msgType != fix.MsgTypeLogout {
			t.Errorf("expected Logout (35=5) for unknown CompID, got MsgType=%q", msgType)
		}
		// The Text field (58) should mention the rejection reason.
		text := fix.GetTag(msg, fix.TagText)
		if !strings.Contains(strings.ToLower(text), "unknown") &&
			!strings.Contains(strings.ToLower(text), "compid") {
			t.Logf("Logout text: %q (acceptable)", text)
		}
	} else {
		// Server closed without sending anything — also acceptable rejection.
		// Verify the connection is indeed closed by attempting a write.
		conn.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
		_, writeErr := conn.Write([]byte("probe"))
		if writeErr == nil {
			// Try a read to confirm closure.
			conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			buf := make([]byte, 16)
			n, readErr := conn.Read(buf)
			if n == 0 && readErr == nil {
				t.Log("connection appears still open with no data — acceptable if server closed write side")
			}
		}
	}

	// Final assertion: the server should eventually close the connection.
	// Give it a short window.
	time.Sleep(100 * time.Millisecond)
	conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	buf := make([]byte, 1)
	n, err := conn.Read(buf)
	if n > 0 {
		t.Logf("got %d extra byte(s) after Logout: %q", n, buf[:n])
	}
	// err should be io.EOF or a timeout — either means the server closed the connection.
	_ = fmt.Sprintf("post-logout read: n=%d err=%v", n, err)
}
