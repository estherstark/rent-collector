package billing

import (
	"errors"
	"fmt"
	"time"
)

// Lease is a rental agreement. Invoices are generated from active leases.
type Lease struct {
	ID       string `json:"id"`
	Tenant   string `json:"tenant"`
	Property string `json:"property"`
	Rent     Money  `json:"rent_satang"`
	DueDay   int    `json:"due_day"` // day of month rent is due (1..31, clamped to month length)
	Active   bool   `json:"active"`
}

// InvoiceStatus is the invoice lifecycle state.
//
//	draft --> issued --> paid
//	             \--> overdue --> paid
type InvoiceStatus string

const (
	StatusDraft   InvoiceStatus = "draft"
	StatusIssued  InvoiceStatus = "issued"
	StatusPaid    InvoiceStatus = "paid"
	StatusOverdue InvoiceStatus = "overdue"
)

// transitions is the single source of truth for legal state changes.
var transitions = map[InvoiceStatus][]InvoiceStatus{
	StatusDraft:   {StatusIssued},
	StatusIssued:  {StatusPaid, StatusOverdue},
	StatusOverdue: {StatusPaid},
	StatusPaid:    {}, // terminal
}

// ErrIllegalTransition is returned (wrapped) for any disallowed state change.
var ErrIllegalTransition = errors.New("illegal invoice state transition")

// ErrNotFound is returned when a lease/invoice does not exist.
var ErrNotFound = errors.New("not found")

// ErrInvalidInput is returned for validation failures.
var ErrInvalidInput = errors.New("invalid input")

// Invoice is a monthly rent bill for one lease.
type Invoice struct {
	ID             string        `json:"id"`
	LeaseID        string        `json:"lease_id"`
	Month          string        `json:"month"` // "2026-08" — with LeaseID forms the idempotency key for generation
	Amount         Money         `json:"amount_satang"`
	LateFee        Money         `json:"late_fee_satang"`
	Paid           Money         `json:"paid_satang"`
	Status         InvoiceStatus `json:"status"`
	DueDate        time.Time     `json:"due_date"`
	LateFeeApplied bool          `json:"late_fee_applied"` // guards the one-time late fee
}

// Total is the full amount owed including late fee.
func (i *Invoice) Total() Money { return i.Amount + i.LateFee }

// Outstanding is what remains to be paid.
func (i *Invoice) Outstanding() Money { return i.Total() - i.Paid }

// TransitionTo enforces the state machine; illegal moves return a typed error.
func (i *Invoice) TransitionTo(next InvoiceStatus) error {
	for _, allowed := range transitions[i.Status] {
		if allowed == next {
			i.Status = next
			return nil
		}
	}
	return fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, i.Status, next)
}

// Payment records money received against an invoice.
// IdempotencyKey lets clients safely retry: the same key always returns
// the original payment instead of recording a duplicate.
type Payment struct {
	ID             string    `json:"id"`
	InvoiceID      string    `json:"invoice_id"`
	Amount         Money     `json:"amount_satang"`
	IdempotencyKey string    `json:"idempotency_key"`
	ReceivedAt     time.Time `json:"received_at"`
}
