package session

import (
	"testing"
	"time"
)

const (
	testSender = "BROKER001"
	testTarget = "EXCHANGE"
	testTenant = "mse-equities"
)

// TestSession_CreateAndGet verifies that a created session can be retrieved by its composite key.
func TestSession_CreateAndGet(t *testing.T) {
	mgr := NewSessionManager()

	s := mgr.CreateSession(testSender, testTarget, testTenant, 30)
	if s == nil {
		t.Fatal("CreateSession returned nil")
	}

	// Verify initial field values.
	if s.SenderCompID != testSender {
		t.Errorf("SenderCompID: got %q, want %q", s.SenderCompID, testSender)
	}
	if s.TargetCompID != testTarget {
		t.Errorf("TargetCompID: got %q, want %q", s.TargetCompID, testTarget)
	}
	if s.TenantID != testTenant {
		t.Errorf("TenantID: got %q, want %q", s.TenantID, testTenant)
	}
	if s.State != Disconnected {
		t.Errorf("State: got %v, want Disconnected", s.State)
	}
	if s.InSeqNum != 1 {
		t.Errorf("InSeqNum: got %d, want 1", s.InSeqNum)
	}
	if s.OutSeqNum != 1 {
		t.Errorf("OutSeqNum: got %d, want 1", s.OutSeqNum)
	}
	if s.HeartbeatInterval != 30 {
		t.Errorf("HeartbeatInterval: got %d, want 30", s.HeartbeatInterval)
	}

	// Retrieve and verify it is the same session.
	got := mgr.GetSession(testSender, testTarget, testTenant)
	if got == nil {
		t.Fatal("GetSession returned nil for a created session")
	}
	if got.SessionKey() != s.SessionKey() {
		t.Errorf("SessionKey mismatch: got %q, want %q", got.SessionKey(), s.SessionKey())
	}
}

// TestSession_GetNonExistent verifies that GetSession returns nil for unknown keys.
func TestSession_GetNonExistent(t *testing.T) {
	mgr := NewSessionManager()
	got := mgr.GetSession("X", "Y", "Z")
	if got != nil {
		t.Errorf("GetSession for non-existent key: got %+v, want nil", got)
	}
}

// TestSession_ProcessLogon_ValidCompID verifies that a valid logon transitions the session to Active.
func TestSession_ProcessLogon(t *testing.T) {
	mgr := NewSessionManager()
	mgr.CreateSession(testSender, testTarget, testTenant, 30)

	err := mgr.ProcessLogon(testSender, testTarget, testTenant)
	if err != nil {
		t.Fatalf("ProcessLogon error: %v", err)
	}

	s := mgr.GetSession(testSender, testTarget, testTenant)
	if s.State != Active {
		t.Errorf("State after logon: got %v, want Active", s.State)
	}
}

// TestSession_ProcessLogon_InvalidCompID verifies that logon on a missing session returns an error.
func TestSession_ProcessLogon_InvalidCompID(t *testing.T) {
	mgr := NewSessionManager()
	// Do NOT create a session — the CompIDs are unknown.

	err := mgr.ProcessLogon("UNKNOWN", "EXCHANGE", testTenant)
	if err == nil {
		t.Fatal("expected error for unknown CompID, got nil")
	}
}

// TestSession_SequenceIncrement verifies that OutSeqNum increments monotonically.
func TestSession_SequenceIncrement(t *testing.T) {
	mgr := NewSessionManager()
	mgr.CreateSession(testSender, testTarget, testTenant, 30)

	// First call should return 1 (the initial value), then OutSeqNum becomes 2.
	seq1, err := mgr.IncrementOutSeq(testSender, testTarget, testTenant)
	if err != nil {
		t.Fatalf("IncrementOutSeq error: %v", err)
	}
	if seq1 != 1 {
		t.Errorf("first seq: got %d, want 1", seq1)
	}

	seq2, err := mgr.IncrementOutSeq(testSender, testTarget, testTenant)
	if err != nil {
		t.Fatalf("IncrementOutSeq error: %v", err)
	}
	if seq2 != 2 {
		t.Errorf("second seq: got %d, want 2", seq2)
	}

	seq3, err := mgr.IncrementOutSeq(testSender, testTarget, testTenant)
	if err != nil {
		t.Fatalf("IncrementOutSeq error: %v", err)
	}
	if seq3 != 3 {
		t.Errorf("third seq: got %d, want 3", seq3)
	}

	// Internal OutSeqNum should now be 4.
	s := mgr.GetSession(testSender, testTarget, testTenant)
	if s.OutSeqNum != 4 {
		t.Errorf("OutSeqNum after 3 increments: got %d, want 4", s.OutSeqNum)
	}
}

// TestSession_SequenceIncrement_UnknownSession verifies error for unknown session.
func TestSession_SequenceIncrement_UnknownSession(t *testing.T) {
	mgr := NewSessionManager()
	_, err := mgr.IncrementOutSeq("X", "Y", "Z")
	if err == nil {
		t.Fatal("expected error for unknown session, got nil")
	}
}

// TestSession_ValidateInSeq_Correct verifies that the correct sequence number is accepted.
func TestSession_ValidateInSeq(t *testing.T) {
	mgr := NewSessionManager()
	mgr.CreateSession(testSender, testTarget, testTenant, 30)

	// Initial InSeqNum is 1; passing 1 should succeed and bump to 2.
	err := mgr.ValidateInSeq(testSender, testTarget, testTenant, 1)
	if err != nil {
		t.Fatalf("ValidateInSeq(1) error: %v", err)
	}

	s := mgr.GetSession(testSender, testTarget, testTenant)
	if s.InSeqNum != 2 {
		t.Errorf("InSeqNum after valid seq: got %d, want 2", s.InSeqNum)
	}

	// Passing 2 should also succeed.
	err = mgr.ValidateInSeq(testSender, testTarget, testTenant, 2)
	if err != nil {
		t.Fatalf("ValidateInSeq(2) error: %v", err)
	}
}

// TestSession_ValidateInSeq_Wrong_TooLow verifies rejection of a sequence number that is too low.
func TestSession_ValidateInSeq_TooLow(t *testing.T) {
	mgr := NewSessionManager()
	mgr.CreateSession(testSender, testTarget, testTenant, 30)

	// Advance InSeqNum to 3.
	_ = mgr.ValidateInSeq(testSender, testTarget, testTenant, 1)
	_ = mgr.ValidateInSeq(testSender, testTarget, testTenant, 2)

	// Now try to send sequence 1 again — too low.
	err := mgr.ValidateInSeq(testSender, testTarget, testTenant, 1)
	if err == nil {
		t.Fatal("expected error for sequence too low, got nil")
	}
}

// TestSession_ValidateInSeq_TooHigh verifies rejection of a sequence gap (too high).
func TestSession_ValidateInSeq_TooHigh(t *testing.T) {
	mgr := NewSessionManager()
	mgr.CreateSession(testSender, testTarget, testTenant, 30)

	// InSeqNum is 1; sending 5 creates a gap.
	err := mgr.ValidateInSeq(testSender, testTarget, testTenant, 5)
	if err == nil {
		t.Fatal("expected error for sequence gap, got nil")
	}
}

// TestSession_ValidateInSeq_UnknownSession verifies error for unknown session.
func TestSession_ValidateInSeq_UnknownSession(t *testing.T) {
	mgr := NewSessionManager()
	err := mgr.ValidateInSeq("X", "Y", "Z", 1)
	if err == nil {
		t.Fatal("expected error for unknown session, got nil")
	}
}

// TestSession_ProcessLogout verifies that logout transitions the session from Active to Disconnected.
func TestSession_ProcessLogout(t *testing.T) {
	mgr := NewSessionManager()
	mgr.CreateSession(testSender, testTarget, testTenant, 30)

	// First log in.
	if err := mgr.ProcessLogon(testSender, testTarget, testTenant); err != nil {
		t.Fatalf("ProcessLogon error: %v", err)
	}

	s := mgr.GetSession(testSender, testTarget, testTenant)
	if s.State != Active {
		t.Fatalf("pre-logout state: got %v, want Active", s.State)
	}

	// Now log out.
	err := mgr.ProcessLogout(testSender, testTarget, testTenant)
	if err != nil {
		t.Fatalf("ProcessLogout error: %v", err)
	}

	if s.State != Disconnected {
		t.Errorf("post-logout state: got %v, want Disconnected", s.State)
	}
}

// TestSession_ProcessLogout_UnknownSession verifies error for unknown session on logout.
func TestSession_ProcessLogout_UnknownSession(t *testing.T) {
	mgr := NewSessionManager()
	err := mgr.ProcessLogout("X", "Y", "Z")
	if err == nil {
		t.Fatal("expected error for unknown session, got nil")
	}
}

// TestSession_ListSessions verifies that ListSessions returns all created sessions.
func TestSession_ListSessions(t *testing.T) {
	mgr := NewSessionManager()

	// Empty manager.
	sessions := mgr.ListSessions()
	if len(sessions) != 0 {
		t.Errorf("ListSessions empty: got %d, want 0", len(sessions))
	}

	// Add three sessions with unique keys.
	mgr.CreateSession("BROKER001", "EXCHANGE", "mse-equities", 30)
	mgr.CreateSession("BROKER002", "EXCHANGE", "mse-equities", 30)
	mgr.CreateSession("BROKER003", "EXCHANGE", "ace-commodities", 30)

	sessions = mgr.ListSessions()
	if len(sessions) != 3 {
		t.Errorf("ListSessions after 3 creates: got %d, want 3", len(sessions))
	}

	// SessionCount should also agree.
	if count := mgr.SessionCount(); count != 3 {
		t.Errorf("SessionCount: got %d, want 3", count)
	}
}

// TestSession_SessionKey verifies the composite key format.
func TestSession_SessionKey(t *testing.T) {
	mgr := NewSessionManager()
	s := mgr.CreateSession("SENDER", "TARGET", "TENANT", 30)

	want := "SENDER:TARGET:TENANT"
	if got := s.SessionKey(); got != want {
		t.Errorf("SessionKey: got %q, want %q", got, want)
	}
}

// TestSession_StateString verifies the String() representation of each SessionState.
func TestSession_StateString(t *testing.T) {
	cases := []struct {
		state SessionState
		want  string
	}{
		{Disconnected, "DISCONNECTED"},
		{LogonSent, "LOGON_SENT"},
		{Active, "ACTIVE"},
		{LogoutSent, "LOGOUT_SENT"},
		{SessionState(99), "UNKNOWN"},
	}

	for _, c := range cases {
		if got := c.state.String(); got != c.want {
			t.Errorf("State %d String(): got %q, want %q", c.state, got, c.want)
		}
	}
}

// TestSession_MarshalState verifies that MarshalState returns the string state.
func TestSession_MarshalState(t *testing.T) {
	mgr := NewSessionManager()
	s := mgr.CreateSession(testSender, testTarget, testTenant, 30)

	if got := s.MarshalState(); got != "DISCONNECTED" {
		t.Errorf("MarshalState: got %q, want DISCONNECTED", got)
	}

	_ = mgr.ProcessLogon(testSender, testTarget, testTenant)
	if got := s.MarshalState(); got != "ACTIVE" {
		t.Errorf("MarshalState after logon: got %q, want ACTIVE", got)
	}
}

// TestSession_UpdateLastRecv verifies that UpdateLastRecv advances the LastRecvTime.
func TestSession_UpdateLastRecv(t *testing.T) {
	mgr := NewSessionManager()
	mgr.CreateSession(testSender, testTarget, testTenant, 30)

	s := mgr.GetSession(testSender, testTarget, testTenant)
	if s == nil {
		t.Fatal("GetSession returned nil")
	}

	before := s.LastRecvTime

	// Small sleep so that the updated time is strictly after before.
	time.Sleep(2 * time.Millisecond)
	mgr.UpdateLastRecv(testSender, testTarget, testTenant)

	after := s.LastRecvTime
	if !after.After(before) {
		t.Errorf("LastRecvTime not updated: before=%v after=%v", before, after)
	}
}

// TestSession_UpdateLastRecv_UnknownSession verifies that UpdateLastRecv is a no-op
// for a session key that does not exist (must not panic).
func TestSession_UpdateLastRecv_UnknownSession(t *testing.T) {
	mgr := NewSessionManager()
	// Should not panic.
	mgr.UpdateLastRecv("NO", "SUCH", "SESSION")
}

// TestSession_RemoveSession verifies that a removed session is no longer retrievable.
func TestSession_RemoveSession(t *testing.T) {
	mgr := NewSessionManager()
	mgr.CreateSession(testSender, testTarget, testTenant, 30)

	// Verify it exists before removal.
	if mgr.GetSession(testSender, testTarget, testTenant) == nil {
		t.Fatal("session not found before RemoveSession")
	}
	if mgr.SessionCount() != 1 {
		t.Fatalf("SessionCount before remove: got %d, want 1", mgr.SessionCount())
	}

	mgr.RemoveSession(testSender, testTarget, testTenant)

	// Must be gone now.
	if got := mgr.GetSession(testSender, testTarget, testTenant); got != nil {
		t.Errorf("GetSession after RemoveSession: got %+v, want nil", got)
	}
	if mgr.SessionCount() != 0 {
		t.Errorf("SessionCount after remove: got %d, want 0", mgr.SessionCount())
	}
}

// TestSession_RemoveSession_UnknownSession verifies that removing a non-existent session
// is a no-op (must not panic).
func TestSession_RemoveSession_UnknownSession(t *testing.T) {
	mgr := NewSessionManager()
	// Should not panic.
	mgr.RemoveSession("NO", "SUCH", "SESSION")
}

// TestSession_ConcurrentAccess verifies the session manager is safe under concurrent use.
func TestSession_ConcurrentAccess(t *testing.T) {
	mgr := NewSessionManager()
	mgr.CreateSession(testSender, testTarget, testTenant, 30)

	done := make(chan struct{})

	// Concurrent reads.
	for i := 0; i < 10; i++ {
		go func() {
			mgr.GetSession(testSender, testTarget, testTenant)
			mgr.ListSessions()
			mgr.SessionCount()
			done <- struct{}{}
		}()
	}

	// Concurrent sequence increments.
	for i := 0; i < 10; i++ {
		go func() {
			mgr.IncrementOutSeq(testSender, testTarget, testTenant)
			done <- struct{}{}
		}()
	}

	for i := 0; i < 20; i++ {
		<-done
	}

	// After 10 concurrent increments starting from OutSeqNum=1, the final value should be 11.
	s := mgr.GetSession(testSender, testTarget, testTenant)
	if s.OutSeqNum != 11 {
		t.Errorf("OutSeqNum after 10 concurrent increments: got %d, want 11", s.OutSeqNum)
	}
}
