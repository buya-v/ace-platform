// Package server — demo reset handler.
package server

import (
	"net/http"
	"reflect"
)

// Resettable is implemented by InMemory stores that support clearing all state.
// Store interfaces are NOT modified; only the concrete InMemory implementations
// carry Reset(). The handler type-asserts each store to Resettable at runtime.
type Resettable interface {
	Reset()
}

// handleDemoReset handles POST /api/v1/securities/demo/reset.
// It clears ALL in-memory state held by the server's stores by calling Reset()
// on every store that implements the Resettable interface.
func (s *Server) handleDemoReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}

	// Collect all store fields via reflection so this list stays in sync
	// automatically when new stores are added to the Server struct.
	stores := []interface{}{
		s.instrumentStore,
		s.orderStore,
		s.tradeStore,
		s.positionStore,
		s.settlementStore,
		s.corporateActionStore,
		s.entitlementStore,
		s.marketStore,
		s.segmentStore,
		s.circuitBreakerStore,
		s.firmStore,
		s.participantStore,
		s.tickTableStore,
		s.tradeCorrectionStore,
		s.throttleStore,
		s.throttleConfigStore,
		s.announcementStore,
		s.auditStore,
		s.pendingChangeStore,
		s.referencePriceStore,
		s.surveillanceStore,
		s.instrumentGroupStore,
		s.offBookTradeStore,
		s.nodeStore,
		s.locateStore,
		s.rfqStore,
		s.giveUpStore,
		s.investigationStore,
		s.replayStore,
		s.bondStore,
		s.strategyStore,
		s.custodyAccountStore,
		s.custodyBalanceStore,
		s.csdTransferStore,
		s.watchListStore,
		s.ipRestrictionStore,
		s.passwordPolicyStore,
		s.tradingParamSetStore,
		s.tradingCycleStore,
		s.historyStore,
		s.postTradeParamsStore,
		s.configTableStore,
		s.clientStore,
		s.indexStore,
		s.entityPermissionStore,
		s.folderStore,
		s.warningStore,
		s.roleStore,
	}

	for _, st := range stores {
		// Skip nil interface values (stores wired after construction via Set* methods
		// may be nil in test environments).
		if st == nil || reflect.ValueOf(st).IsNil() {
			continue
		}
		if r, ok := st.(Resettable); ok {
			r.Reset()
		}
	}

	// Also clear PostgreSQL tables if a database connection is available.
	if s.db != nil {
		tables := []string{
			"ace_securities.trades",
			"ace_securities.orders",
			"ace_securities.settlement_obligations",
			"ace_securities.positions",
			"ace_securities.instruments",
			"ace_securities.audit_log",
		}
		for _, table := range tables {
			s.db.Exec("DELETE FROM " + table)
		}
	}

	// Reset the day manager state back to CLOSED.
	if s.dayManager != nil {
		s.dayManager.Reset()
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "reset",
		"message": "All securities data cleared",
	})
}
