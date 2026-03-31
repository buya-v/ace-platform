package dvp

import (
	"fmt"
	"time"

	"github.com/garudax-platform/settlement-engine/internal/types"
)

// DeliveryValidator checks whether a delivery receipt is valid and confirms delivery.
type DeliveryValidator interface {
	// ValidateReceipt checks that a delivery receipt exists and is well-formed.
	ValidateReceipt(receipt types.DeliveryReceipt) error
	// ConfirmDelivery marks the commodity as transferred to the buyer.
	ConfirmDelivery(receipt types.DeliveryReceipt) (types.DeliveryReceipt, error)
	// RollbackDelivery reverses a confirmed delivery (e.g., returns goods to seller).
	RollbackDelivery(receipt types.DeliveryReceipt) error
}

// PaymentLocker can lock and release funds for DVP settlement.
type PaymentLocker interface {
	// LockFunds places a hold on the buyer's payment amount.
	LockFunds(participantID string, amount types.Decimal) (lockID string, err error)
	// ReleaseFunds confirms the locked payment to the seller.
	ReleaseFunds(lockID string) error
	// UnlockFunds releases the hold without transferring (rollback).
	UnlockFunds(lockID string) error
}

// DVPCoordinator orchestrates delivery-vs-payment settlement.
// For physical delivery instruments, it ensures commodity delivery and cash payment
// happen atomically: validate delivery receipt -> lock payment -> confirm delivery -> release payment.
// If either side fails, both are rolled back.
// For cash-settled instruments, it delegates to standard cash flow processing.
type DVPCoordinator struct {
	validator DeliveryValidator
	locker    PaymentLocker
	idGen     types.IDGenerator
}

// NewDVPCoordinator creates a new DVP coordinator.
func NewDVPCoordinator(validator DeliveryValidator, locker PaymentLocker, idGen types.IDGenerator) *DVPCoordinator {
	return &DVPCoordinator{
		validator: validator,
		locker:    locker,
		idGen:     idGen,
	}
}

// CoordinateSettlement runs DVP coordination for a set of settlement instructions and delivery receipts.
// For physical delivery instruments, it ensures atomic delivery-vs-payment.
// For cash-settled instruments, it simply passes through the instructions unchanged.
func (c *DVPCoordinator) CoordinateSettlement(
	cycleID string,
	instrumentConfig types.InstrumentConfig,
	instructions []types.SettlementInstruction,
	receipts []types.DeliveryReceipt,
) *types.DVPResult {
	result := &types.DVPResult{
		InstrumentID: instrumentConfig.InstrumentID,
		CompletedAt:  time.Now(),
	}

	if instrumentConfig.Type == types.InstrumentCashSettled {
		// Cash-settled: no delivery coordination needed, just pass through instructions.
		result.Status = types.DVPSucceeded
		result.Instructions = instructions
		return result
	}

	// Physical delivery: run the DVP sequence for each receipt.
	if len(receipts) == 0 {
		result.Status = types.DVPFailed
		result.FailedAtStep = types.DVPStepValidateDelivery
		result.Error = "no delivery receipts provided for physical delivery instrument"
		return result
	}

	confirmedReceipts := make([]types.DeliveryReceipt, 0, len(receipts))
	lockIDs := make([]string, 0, len(receipts))

	for _, receipt := range receipts {
		// Step 1: Validate delivery receipt
		result.FailedAtStep = types.DVPStepValidateDelivery
		if err := c.validator.ValidateReceipt(receipt); err != nil {
			result.Status = types.DVPFailed
			result.Error = fmt.Sprintf("delivery validation failed for receipt %s: %v", receipt.ReceiptID, err)
			c.rollbackAll(confirmedReceipts, lockIDs)
			result.Status = types.DVPRolledBack
			return result
		}

		// Step 2: Lock buyer's payment
		result.FailedAtStep = types.DVPStepLockPayment
		payAmount := findPaymentAmount(instructions, receipt.BuyerID)
		lockID, err := c.locker.LockFunds(receipt.BuyerID, payAmount)
		if err != nil {
			result.Status = types.DVPFailed
			result.Error = fmt.Sprintf("payment lock failed for buyer %s: %v", receipt.BuyerID, err)
			c.rollbackAll(confirmedReceipts, lockIDs)
			result.Status = types.DVPRolledBack
			return result
		}
		lockIDs = append(lockIDs, lockID)

		// Step 3: Confirm delivery
		result.FailedAtStep = types.DVPStepConfirmDelivery
		confirmed, err := c.validator.ConfirmDelivery(receipt)
		if err != nil {
			result.Status = types.DVPFailed
			result.Error = fmt.Sprintf("delivery confirmation failed for receipt %s: %v", receipt.ReceiptID, err)
			c.rollbackAll(confirmedReceipts, lockIDs)
			result.Status = types.DVPRolledBack
			return result
		}
		confirmedReceipts = append(confirmedReceipts, confirmed)

		// Step 4: Release payment to seller
		result.FailedAtStep = types.DVPStepReleasePayment
		if err := c.locker.ReleaseFunds(lockID); err != nil {
			result.Status = types.DVPFailed
			result.Error = fmt.Sprintf("payment release failed for lock %s: %v", lockID, err)
			// Rollback the confirmed delivery too
			c.rollbackAll(confirmedReceipts, lockIDs)
			result.Status = types.DVPRolledBack
			return result
		}
	}

	// All receipts processed successfully
	result.Status = types.DVPSucceeded
	result.DeliveryReceipts = confirmedReceipts
	result.Instructions = markInstructionsConfirmed(instructions)
	result.CompletedAt = time.Now()
	return result
}

// rollbackAll reverses all confirmed deliveries and unlocks all locked funds.
func (c *DVPCoordinator) rollbackAll(confirmedReceipts []types.DeliveryReceipt, lockIDs []string) {
	for _, receipt := range confirmedReceipts {
		_ = c.validator.RollbackDelivery(receipt) // best-effort rollback
	}
	for _, lockID := range lockIDs {
		_ = c.locker.UnlockFunds(lockID) // best-effort unlock
	}
}

// findPaymentAmount finds the payment amount for a given participant from the instructions.
func findPaymentAmount(instructions []types.SettlementInstruction, participantID string) types.Decimal {
	for _, inst := range instructions {
		if inst.ParticipantID == participantID {
			return inst.Amount
		}
	}
	return types.DecimalZero()
}

// markInstructionsConfirmed returns a copy of instructions with status set to Confirmed.
func markInstructionsConfirmed(instructions []types.SettlementInstruction) []types.SettlementInstruction {
	result := make([]types.SettlementInstruction, len(instructions))
	now := time.Now()
	for i, inst := range instructions {
		inst.Status = types.InstructionConfirmed
		inst.ConfirmedAt = now
		result[i] = inst
	}
	return result
}
