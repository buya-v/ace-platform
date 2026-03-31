package dvp

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garudax-platform/settlement-engine/internal/types"
)

// --- Test doubles ---

type seqIDGen struct {
	counter uint64
}

func (g *seqIDGen) NewID() string {
	n := atomic.AddUint64(&g.counter, 1)
	return fmt.Sprintf("test-%d", n)
}

// inMemoryDeliveryValidator simulates delivery validation and confirmation.
type inMemoryDeliveryValidator struct {
	validateErr  map[string]error // receiptID -> error
	confirmErr   map[string]error
	rollbackErr  map[string]error
	confirmed    []types.DeliveryReceipt
	rolledBack   []types.DeliveryReceipt
}

func newInMemoryDeliveryValidator() *inMemoryDeliveryValidator {
	return &inMemoryDeliveryValidator{
		validateErr: make(map[string]error),
		confirmErr:  make(map[string]error),
		rollbackErr: make(map[string]error),
	}
}

func (v *inMemoryDeliveryValidator) ValidateReceipt(receipt types.DeliveryReceipt) error {
	if err, ok := v.validateErr[receipt.ReceiptID]; ok {
		return err
	}
	return nil
}

func (v *inMemoryDeliveryValidator) ConfirmDelivery(receipt types.DeliveryReceipt) (types.DeliveryReceipt, error) {
	if err, ok := v.confirmErr[receipt.ReceiptID]; ok {
		return receipt, err
	}
	receipt.Status = types.DeliveryConfirmed
	receipt.ConfirmedAt = time.Now()
	v.confirmed = append(v.confirmed, receipt)
	return receipt, nil
}

func (v *inMemoryDeliveryValidator) RollbackDelivery(receipt types.DeliveryReceipt) error {
	if err, ok := v.rollbackErr[receipt.ReceiptID]; ok {
		return err
	}
	v.rolledBack = append(v.rolledBack, receipt)
	return nil
}

// inMemoryPaymentLocker simulates fund locking and release.
type inMemoryPaymentLocker struct {
	lockCounter uint64
	lockErr     map[string]error  // participantID -> error
	releaseErr  map[string]error  // lockID -> error
	locked      map[string]bool   // lockID -> locked
	unlocked    []string          // unlocked lockIDs
}

func newInMemoryPaymentLocker() *inMemoryPaymentLocker {
	return &inMemoryPaymentLocker{
		lockErr:    make(map[string]error),
		releaseErr: make(map[string]error),
		locked:     make(map[string]bool),
	}
}

func (l *inMemoryPaymentLocker) LockFunds(participantID string, amount types.Decimal) (string, error) {
	if err, ok := l.lockErr[participantID]; ok {
		return "", err
	}
	n := atomic.AddUint64(&l.lockCounter, 1)
	lockID := fmt.Sprintf("lock-%d", n)
	l.locked[lockID] = true
	return lockID, nil
}

func (l *inMemoryPaymentLocker) ReleaseFunds(lockID string) error {
	if err, ok := l.releaseErr[lockID]; ok {
		return err
	}
	delete(l.locked, lockID)
	return nil
}

func (l *inMemoryPaymentLocker) UnlockFunds(lockID string) error {
	delete(l.locked, lockID)
	l.unlocked = append(l.unlocked, lockID)
	return nil
}

// --- Helper ---

func makeReceipt(id, instrumentID, sellerID, buyerID string, qty int64) types.DeliveryReceipt {
	return types.DeliveryReceipt{
		ReceiptID:    id,
		InstrumentID: instrumentID,
		SellerID:     sellerID,
		BuyerID:      buyerID,
		Quantity:     qty,
		WarehouseID:  "WH-1",
		Status:       types.DeliveryPending,
		IssuedAt:     time.Now(),
	}
}

func makeInstruction(id, cycleID, participantID string, dir types.PayDirection, amount types.Decimal) types.SettlementInstruction {
	return types.SettlementInstruction{
		InstructionID: id,
		CycleID:       cycleID,
		ParticipantID: participantID,
		Direction:     dir,
		Amount:        amount,
		Status:        types.InstructionPending,
		CreatedAt:     time.Now(),
	}
}

// --- Tests ---

func TestDVPCashSettlement(t *testing.T) {
	validator := newInMemoryDeliveryValidator()
	locker := newInMemoryPaymentLocker()
	coord := NewDVPCoordinator(validator, locker, &seqIDGen{})

	config := types.InstrumentConfig{
		InstrumentID: "WHEAT-MAY26",
		Type:         types.InstrumentCashSettled,
	}

	instructions := []types.SettlementInstruction{
		makeInstruction("inst-1", "cycle-1", "P1", types.PayOut, types.NewDecimal(200, 0)),
		makeInstruction("inst-2", "cycle-1", "P2", types.PayIn, types.NewDecimal(200, 0)),
	}

	result := coord.CoordinateSettlement("cycle-1", config, instructions, nil)

	if result.Status != types.DVPSucceeded {
		t.Errorf("expected SUCCEEDED, got %s", result.Status)
	}
	if len(result.Instructions) != 2 {
		t.Errorf("expected 2 instructions, got %d", len(result.Instructions))
	}
	// Cash settlement should not touch delivery validator at all
	if len(validator.confirmed) != 0 {
		t.Errorf("expected no deliveries confirmed for cash settlement, got %d", len(validator.confirmed))
	}
}

func TestDVPPhysicalDeliverySuccess(t *testing.T) {
	validator := newInMemoryDeliveryValidator()
	locker := newInMemoryPaymentLocker()
	coord := NewDVPCoordinator(validator, locker, &seqIDGen{})

	config := types.InstrumentConfig{
		InstrumentID: "GOLD-SPOT",
		Type:         types.InstrumentPhysicalDelivery,
		ContractUnit: "OZ",
		ContractSize: 100,
	}

	instructions := []types.SettlementInstruction{
		makeInstruction("inst-1", "cycle-1", "BUYER-1", types.PayIn, types.NewDecimal(50000, 0)),
		makeInstruction("inst-2", "cycle-1", "SELLER-1", types.PayOut, types.NewDecimal(50000, 0)),
	}

	receipts := []types.DeliveryReceipt{
		makeReceipt("receipt-1", "GOLD-SPOT", "SELLER-1", "BUYER-1", 100),
	}

	result := coord.CoordinateSettlement("cycle-1", config, instructions, receipts)

	if result.Status != types.DVPSucceeded {
		t.Errorf("expected SUCCEEDED, got %s (error: %s)", result.Status, result.Error)
	}
	if len(result.DeliveryReceipts) != 1 {
		t.Errorf("expected 1 delivery receipt, got %d", len(result.DeliveryReceipts))
	}
	if result.DeliveryReceipts[0].Status != types.DeliveryConfirmed {
		t.Errorf("expected delivery CONFIRMED, got %s", result.DeliveryReceipts[0].Status)
	}
	// All instructions should be confirmed
	for _, inst := range result.Instructions {
		if inst.Status != types.InstructionConfirmed {
			t.Errorf("expected instruction CONFIRMED, got %s", inst.Status)
		}
	}
	// No locks should remain
	if len(locker.locked) != 0 {
		t.Errorf("expected no outstanding locks, got %d", len(locker.locked))
	}
}

func TestDVPDeliveryValidationFailureRollback(t *testing.T) {
	validator := newInMemoryDeliveryValidator()
	locker := newInMemoryPaymentLocker()
	coord := NewDVPCoordinator(validator, locker, &seqIDGen{})

	config := types.InstrumentConfig{
		InstrumentID: "GOLD-SPOT",
		Type:         types.InstrumentPhysicalDelivery,
	}

	instructions := []types.SettlementInstruction{
		makeInstruction("inst-1", "cycle-1", "BUYER-1", types.PayIn, types.NewDecimal(50000, 0)),
	}

	// Set validation to fail
	validator.validateErr["receipt-1"] = fmt.Errorf("receipt expired")

	receipts := []types.DeliveryReceipt{
		makeReceipt("receipt-1", "GOLD-SPOT", "SELLER-1", "BUYER-1", 100),
	}

	result := coord.CoordinateSettlement("cycle-1", config, instructions, receipts)

	if result.Status != types.DVPRolledBack {
		t.Errorf("expected ROLLED_BACK, got %s", result.Status)
	}
	if result.FailedAtStep != types.DVPStepValidateDelivery {
		t.Errorf("expected failure at VALIDATE_DELIVERY, got %s", result.FailedAtStep)
	}
	if result.Error == "" {
		t.Error("expected error message")
	}
}

func TestDVPPaymentLockFailureRollback(t *testing.T) {
	validator := newInMemoryDeliveryValidator()
	locker := newInMemoryPaymentLocker()
	coord := NewDVPCoordinator(validator, locker, &seqIDGen{})

	config := types.InstrumentConfig{
		InstrumentID: "GOLD-SPOT",
		Type:         types.InstrumentPhysicalDelivery,
	}

	instructions := []types.SettlementInstruction{
		makeInstruction("inst-1", "cycle-1", "BUYER-1", types.PayIn, types.NewDecimal(50000, 0)),
	}

	// Set lock to fail
	locker.lockErr["BUYER-1"] = fmt.Errorf("insufficient funds")

	receipts := []types.DeliveryReceipt{
		makeReceipt("receipt-1", "GOLD-SPOT", "SELLER-1", "BUYER-1", 100),
	}

	result := coord.CoordinateSettlement("cycle-1", config, instructions, receipts)

	if result.Status != types.DVPRolledBack {
		t.Errorf("expected ROLLED_BACK, got %s", result.Status)
	}
	if result.FailedAtStep != types.DVPStepLockPayment {
		t.Errorf("expected failure at LOCK_PAYMENT, got %s", result.FailedAtStep)
	}
}

func TestDVPDeliveryConfirmFailureRollback(t *testing.T) {
	validator := newInMemoryDeliveryValidator()
	locker := newInMemoryPaymentLocker()
	coord := NewDVPCoordinator(validator, locker, &seqIDGen{})

	config := types.InstrumentConfig{
		InstrumentID: "GOLD-SPOT",
		Type:         types.InstrumentPhysicalDelivery,
	}

	instructions := []types.SettlementInstruction{
		makeInstruction("inst-1", "cycle-1", "BUYER-1", types.PayIn, types.NewDecimal(50000, 0)),
	}

	// Delivery confirmation fails
	validator.confirmErr["receipt-1"] = fmt.Errorf("warehouse rejected delivery")

	receipts := []types.DeliveryReceipt{
		makeReceipt("receipt-1", "GOLD-SPOT", "SELLER-1", "BUYER-1", 100),
	}

	result := coord.CoordinateSettlement("cycle-1", config, instructions, receipts)

	if result.Status != types.DVPRolledBack {
		t.Errorf("expected ROLLED_BACK, got %s", result.Status)
	}
	if result.FailedAtStep != types.DVPStepConfirmDelivery {
		t.Errorf("expected failure at CONFIRM_DELIVERY, got %s", result.FailedAtStep)
	}
	// Lock should have been unlocked during rollback
	if len(locker.unlocked) != 1 {
		t.Errorf("expected 1 unlock (rollback), got %d", len(locker.unlocked))
	}
}

func TestDVPPaymentReleaseFailureRollback(t *testing.T) {
	validator := newInMemoryDeliveryValidator()
	locker := newInMemoryPaymentLocker()
	coord := NewDVPCoordinator(validator, locker, &seqIDGen{})

	config := types.InstrumentConfig{
		InstrumentID: "GOLD-SPOT",
		Type:         types.InstrumentPhysicalDelivery,
	}

	instructions := []types.SettlementInstruction{
		makeInstruction("inst-1", "cycle-1", "BUYER-1", types.PayIn, types.NewDecimal(50000, 0)),
	}

	// Release will fail — we need to know the lockID to set the error.
	// Since lockID is generated inside CoordinateSettlement, we set it to fail for "lock-1".
	locker.releaseErr["lock-1"] = fmt.Errorf("payment network timeout")

	receipts := []types.DeliveryReceipt{
		makeReceipt("receipt-1", "GOLD-SPOT", "SELLER-1", "BUYER-1", 100),
	}

	result := coord.CoordinateSettlement("cycle-1", config, instructions, receipts)

	if result.Status != types.DVPRolledBack {
		t.Errorf("expected ROLLED_BACK, got %s", result.Status)
	}
	if result.FailedAtStep != types.DVPStepReleasePayment {
		t.Errorf("expected failure at RELEASE_PAYMENT, got %s", result.FailedAtStep)
	}
	// Delivery should have been rolled back
	if len(validator.rolledBack) != 1 {
		t.Errorf("expected 1 delivery rollback, got %d", len(validator.rolledBack))
	}
}

func TestDVPNoReceiptsForPhysicalDelivery(t *testing.T) {
	validator := newInMemoryDeliveryValidator()
	locker := newInMemoryPaymentLocker()
	coord := NewDVPCoordinator(validator, locker, &seqIDGen{})

	config := types.InstrumentConfig{
		InstrumentID: "GOLD-SPOT",
		Type:         types.InstrumentPhysicalDelivery,
	}

	instructions := []types.SettlementInstruction{
		makeInstruction("inst-1", "cycle-1", "BUYER-1", types.PayIn, types.NewDecimal(50000, 0)),
	}

	result := coord.CoordinateSettlement("cycle-1", config, instructions, nil)

	if result.Status != types.DVPFailed {
		t.Errorf("expected FAILED, got %s", result.Status)
	}
	if result.FailedAtStep != types.DVPStepValidateDelivery {
		t.Errorf("expected failure at VALIDATE_DELIVERY, got %s", result.FailedAtStep)
	}
}

func TestDVPMultipleReceipts(t *testing.T) {
	validator := newInMemoryDeliveryValidator()
	locker := newInMemoryPaymentLocker()
	coord := NewDVPCoordinator(validator, locker, &seqIDGen{})

	config := types.InstrumentConfig{
		InstrumentID: "WHEAT-PHYS",
		Type:         types.InstrumentPhysicalDelivery,
		ContractUnit: "MT",
		ContractSize: 50,
	}

	instructions := []types.SettlementInstruction{
		makeInstruction("inst-1", "cycle-1", "BUYER-1", types.PayIn, types.NewDecimal(30000, 0)),
		makeInstruction("inst-2", "cycle-1", "BUYER-2", types.PayIn, types.NewDecimal(20000, 0)),
		makeInstruction("inst-3", "cycle-1", "SELLER-1", types.PayOut, types.NewDecimal(50000, 0)),
	}

	receipts := []types.DeliveryReceipt{
		makeReceipt("receipt-1", "WHEAT-PHYS", "SELLER-1", "BUYER-1", 50),
		makeReceipt("receipt-2", "WHEAT-PHYS", "SELLER-1", "BUYER-2", 50),
	}

	result := coord.CoordinateSettlement("cycle-1", config, instructions, receipts)

	if result.Status != types.DVPSucceeded {
		t.Errorf("expected SUCCEEDED, got %s (error: %s)", result.Status, result.Error)
	}
	if len(result.DeliveryReceipts) != 2 {
		t.Errorf("expected 2 delivery receipts, got %d", len(result.DeliveryReceipts))
	}
	if len(result.Instructions) != 3 {
		t.Errorf("expected 3 instructions, got %d", len(result.Instructions))
	}
}

func TestDVPSecondReceiptFailureRollsBackFirst(t *testing.T) {
	validator := newInMemoryDeliveryValidator()
	locker := newInMemoryPaymentLocker()
	coord := NewDVPCoordinator(validator, locker, &seqIDGen{})

	config := types.InstrumentConfig{
		InstrumentID: "WHEAT-PHYS",
		Type:         types.InstrumentPhysicalDelivery,
	}

	instructions := []types.SettlementInstruction{
		makeInstruction("inst-1", "cycle-1", "BUYER-1", types.PayIn, types.NewDecimal(30000, 0)),
		makeInstruction("inst-2", "cycle-1", "BUYER-2", types.PayIn, types.NewDecimal(20000, 0)),
	}

	// Second receipt fails validation
	validator.validateErr["receipt-2"] = fmt.Errorf("invalid grade certificate")

	receipts := []types.DeliveryReceipt{
		makeReceipt("receipt-1", "WHEAT-PHYS", "SELLER-1", "BUYER-1", 50),
		makeReceipt("receipt-2", "WHEAT-PHYS", "SELLER-1", "BUYER-2", 50),
	}

	result := coord.CoordinateSettlement("cycle-1", config, instructions, receipts)

	if result.Status != types.DVPRolledBack {
		t.Errorf("expected ROLLED_BACK, got %s", result.Status)
	}
	// First receipt's delivery should be rolled back
	if len(validator.rolledBack) != 1 {
		t.Errorf("expected 1 delivery rollback (first receipt), got %d", len(validator.rolledBack))
	}
	// First receipt's lock should be unlocked
	if len(locker.unlocked) != 1 {
		t.Errorf("expected 1 unlock (first receipt's lock), got %d", len(locker.unlocked))
	}
}
