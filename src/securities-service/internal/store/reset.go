// Package store — Reset() methods for all InMemory store implementations.
// These methods are NOT part of the store interfaces; they implement the
// server.Resettable interface, which the demo-reset handler type-asserts at
// runtime via the Resettable interface defined in the server package.
package store

import "github.com/garudax-platform/securities-service/internal/types"

// Reset clears all participants.
func (s *InMemoryParticipantStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.ExchangeParticipant)
	s.mu.Unlock()
}

// Reset clears all firms.
func (s *InMemoryFirmStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.Firm)
	s.mu.Unlock()
}

// Reset clears all instruments.
func (s *InMemoryInstrumentStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.Instrument)
	s.mu.Unlock()
}

// Reset clears all orders.
func (s *InMemoryOrderStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.SecurityOrder)
	s.mu.Unlock()
}

// Reset clears all trades.
func (s *InMemoryTradeStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.SecurityTrade)
	s.mu.Unlock()
}

// Reset clears all positions.
func (s *InMemoryPositionStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.Position)
	s.mu.Unlock()
}

// Reset clears all settlement obligations.
func (s *InMemorySettlementStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.SettlementObligation)
	s.mu.Unlock()
}

// Reset clears all corporate actions.
func (s *InMemoryCorporateActionStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.CorporateAction)
	s.mu.Unlock()
}

// Reset clears all entitlements.
func (s *InMemoryEntitlementStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.Entitlement)
	s.mu.Unlock()
}

// Reset clears all markets.
func (s *InMemoryMarketStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.Market)
	s.mu.Unlock()
}

// Reset clears all segments.
func (s *InMemorySegmentStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.Segment)
	s.mu.Unlock()
}

// Reset clears all circuit breakers.
func (s *InMemoryCircuitBreakerStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.CircuitBreaker)
	s.mu.Unlock()
}

// Reset clears all trade corrections.
func (s *InMemoryTradeCorrectionStore) Reset() {
	s.mu.Lock()
	s.data = nil
	s.mu.Unlock()
}

// Reset clears all tick tables.
func (s *InMemoryTickTableStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.TickTable)
	s.mu.Unlock()
}

// Reset clears all announcements.
func (s *InMemoryAnnouncementStore) Reset() {
	s.mu.Lock()
	s.data = nil
	s.mu.Unlock()
}

// Reset clears all audit entries.
func (s *InMemoryAuditStore) Reset() {
	s.mu.Lock()
	s.data = nil
	s.mu.Unlock()
}

// Reset clears all throttle buckets.
func (s *InMemoryThrottleStore) Reset() {
	s.mu.Lock()
	s.buckets = make(map[string]*throttleBucket)
	s.mu.Unlock()
}

// Reset clears all pending changes.
func (s *InMemoryPendingChangeStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.PendingChange)
	s.mu.Unlock()
}

// Reset clears all reference prices.
func (s *InMemoryReferencePriceStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.ReferencePrice)
	s.mu.Unlock()
}

// Reset clears all surveillance alerts and thresholds.
func (s *InMemorySurveillanceStore) Reset() {
	s.mu.Lock()
	s.alerts = make(map[string]*types.SurveillanceAlert)
	s.thresholds = make(map[string]*types.SurveillanceThreshold)
	s.mu.Unlock()
}

// Reset clears all instrument groups.
func (s *InMemoryInstrumentGroupStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.InstrumentGroup)
	s.mu.Unlock()
}

// Reset clears all off-book trades.
func (s *InMemoryOffBookTradeStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.OffBookTrade)
	s.mu.Unlock()
}

// Reset clears all nodes.
func (s *InMemoryNodeStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.Node)
	s.mu.Unlock()
}

// Reset clears all locate requests and resets the sequence counter.
func (s *InMemoryLocateStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.LocateRequest)
	s.nextID = 1
	s.mu.Unlock()
}

// Reset clears all RFQs and resets the sequence counter.
func (s *InMemoryRFQStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.RequestForQuote)
	s.nextID = 1
	s.mu.Unlock()
}

// Reset clears all give-up requests and resets the sequence counter.
func (s *InMemoryGiveUpStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.GiveUpRequest)
	s.nextID = 1
	s.mu.Unlock()
}

// Reset clears all investigations.
func (s *InMemoryInvestigationStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.Investigation)
	s.mu.Unlock()
}

// Reset clears all replay sessions and events.
func (s *InMemoryReplayStore) Reset() {
	s.mu.Lock()
	s.sessions = make(map[string]*types.ReplaySession)
	s.events = nil
	s.mu.Unlock()
}

// Reset clears all bonds.
func (s *InMemoryBondStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.Bond)
	s.mu.Unlock()
}

// Reset clears all trading strategies.
func (s *InMemoryStrategyStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.TradingStrategy)
	s.mu.Unlock()
}

// Reset clears all custody accounts.
func (s *InMemoryCustodyAccountStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.CustodyAccount)
	s.mu.Unlock()
}

// Reset clears all custody balances.
func (s *InMemoryCustodyBalanceStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.CustodyBalance)
	s.mu.Unlock()
}

// Reset clears all CSD transfers.
func (s *InMemoryCSDTransferStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.CSDTransfer)
	s.mu.Unlock()
}

// Reset clears all throttle configs.
func (s *InMemoryThrottleConfigStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.ThrottleConfig)
	s.mu.Unlock()
}

// Reset clears all watch lists.
func (s *InMemoryWatchListStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.WatchList)
	s.mu.Unlock()
}

// Reset clears all IP restrictions.
func (s *InMemoryIPRestrictionStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.IPRestriction)
	s.mu.Unlock()
}

// Reset clears all password policies.
func (s *InMemoryPasswordPolicyStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.PasswordPolicy)
	s.mu.Unlock()
}

// Reset clears all trading parameter sets (both data and the instrument index).
func (s *InMemoryTradingParamSetStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.TradingParameterSet)
	s.byInstrument = make(map[string]string)
	s.mu.Unlock()
}

// Reset clears all trading cycles.
func (s *InMemoryTradingCycleStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.TradingCycle)
	s.mu.Unlock()
}

// Reset clears all history (archived orders and trades).
func (s *InMemoryHistoryStore) Reset() {
	s.mu.Lock()
	s.orders = nil
	s.trades = nil
	s.mu.Unlock()
}

// Reset clears all post-trade parameter records.
func (s *InMemoryPostTradeParamsStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.PostTradeParams)
	s.mu.Unlock()
}

// Reset clears all config tables.
func (s *InMemoryConfigTableStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.ConfigTable)
	s.mu.Unlock()
}

// Reset clears all clients.
func (s *InMemoryClientStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.Client)
	s.mu.Unlock()
}

// Reset clears all indices.
func (s *InMemoryIndexStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.Index)
	s.mu.Unlock()
}

// Reset clears all entity permissions.
func (s *InMemoryEntityPermissionStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.EntityPermission)
	s.mu.Unlock()
}

// Reset clears all folders.
func (s *InMemoryFolderStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.Folder)
	s.mu.Unlock()
}

// Reset clears all warnings.
func (s *InMemoryWarningStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.Warning)
	s.mu.Unlock()
}

// Reset clears all roles.
func (s *InMemoryRoleStore) Reset() {
	s.mu.Lock()
	s.data = make(map[string]*types.Role)
	s.mu.Unlock()
}
