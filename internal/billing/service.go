package billing

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Repositories are defined here, by the consumer (idiomatic Go).
// The in-memory implementations live in internal/store; Postgres is next —
// there, unique constraints (lease_id+month, idempotency_key) enforce
// idempotency at the DB level instead of the service mutex below.

type LeaseRepo interface {
	Save(l *Lease) error
	Get(id string) (*Lease, error)
	ListActive() ([]*Lease, error)
	List() ([]*Lease, error)
}

type InvoiceRepo interface {
	Save(inv *Invoice) error
	Get(id string) (*Invoice, error)
	GetByLeaseMonth(leaseID, month string) (*Invoice, error)
	ListByStatus(status InvoiceStatus) ([]*Invoice, error)
	List(month string) ([]*Invoice, error)
}

type PaymentRepo interface {
	Save(p *Payment) error
	GetByKey(key string) (*Payment, error)
}

// Config holds billing policy knobs.
type Config struct {
	LateFee Money // one-time flat late fee; default 500 THB
}

// DefaultConfig returns the MVP policy: flat 500 THB late fee.
func DefaultConfig() Config { return Config{LateFee: THB(500)} }

// Service is the billing engine. A single mutex serializes commands so the
// check-then-insert idempotency logic is race-free; with Postgres this is
// replaced by unique constraints + transactions.
type Service struct {
	mu       sync.Mutex
	cfg      Config
	leases   LeaseRepo
	invoices InvoiceRepo
	payments PaymentRepo
}

func NewService(cfg Config, l LeaseRepo, i InvoiceRepo, p PaymentRepo) *Service {
	return &Service{cfg: cfg, leases: l, invoices: i, payments: p}
}

// CreateLease validates and stores a new active lease.
func (s *Service) CreateLease(tenant, property string, rent Money, dueDay int) (*Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if tenant == "" || property == "" {
		return nil, fmt.Errorf("%w: tenant and property are required", ErrInvalidInput)
	}
	if rent <= 0 {
		return nil, fmt.Errorf("%w: rent must be positive", ErrInvalidInput)
	}
	if dueDay < 1 || dueDay > 31 {
		return nil, fmt.Errorf("%w: due_day must be 1..31", ErrInvalidInput)
	}
	l := &Lease{ID: uuid.NewString(), Tenant: tenant, Property: property, Rent: rent, DueDay: dueDay, Active: true}
	if err := s.leases.Save(l); err != nil {
		return nil, err
	}
	return l, nil
}

func (s *Service) ListLeases() ([]*Lease, error) { return s.leases.List() }

// GenerateResult reports what a generation run did — created vs skipped
// makes idempotent re-runs observable.
type GenerateResult struct {
	Month   string `json:"month"`
	Created int    `json:"created"`
	Skipped int    `json:"skipped"`
}

// GenerateInvoices creates one issued invoice per active lease for the given
// month ("2026-08"). IDEMPOTENT: (lease, month) is a unique key, so calling
// twice creates nothing new — existing invoices are counted as skipped.
func (s *Service) GenerateInvoices(month string) (*GenerateResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	monthStart, err := time.Parse("2006-01", month)
	if err != nil {
		return nil, fmt.Errorf("%w: month must be YYYY-MM", ErrInvalidInput)
	}
	leases, err := s.leases.ListActive()
	if err != nil {
		return nil, err
	}

	res := &GenerateResult{Month: month}
	for _, l := range leases {
		if existing, err := s.invoices.GetByLeaseMonth(l.ID, month); err != nil {
			return nil, err
		} else if existing != nil {
			res.Skipped++
			continue
		}
		inv := &Invoice{
			ID:      uuid.NewString(),
			LeaseID: l.ID,
			Month:   month,
			Amount:  l.Rent,
			Status:  StatusDraft,
			DueDate: dueDate(monthStart, l.DueDay),
		}
		if err := inv.TransitionTo(StatusIssued); err != nil {
			return nil, err // unreachable: draft -> issued is always legal
		}
		if err := s.invoices.Save(inv); err != nil {
			return nil, err
		}
		res.Created++
	}
	return res, nil
}

// dueDate clamps dueDay to the last day of the month (e.g. day 31 in Feb).
func dueDate(monthStart time.Time, dueDay int) time.Time {
	lastDay := monthStart.AddDate(0, 1, -1).Day()
	if dueDay > lastDay {
		dueDay = lastDay
	}
	return time.Date(monthStart.Year(), monthStart.Month(), dueDay, 0, 0, 0, 0, time.UTC)
}

// LateFeeResult reports how many invoices became overdue.
type LateFeeResult struct {
	MarkedOverdue int `json:"marked_overdue"`
}

// ApplyLateFees marks issued invoices past their due date as overdue and adds
// a ONE-TIME flat late fee. Safe to re-run: already-overdue invoices are no
// longer in issued state, and LateFeeApplied guards against double-charging.
func (s *Service) ApplyLateFees(asOf time.Time) (*LateFeeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	issued, err := s.invoices.ListByStatus(StatusIssued)
	if err != nil {
		return nil, err
	}
	res := &LateFeeResult{}
	for _, inv := range issued {
		if !asOf.After(inv.DueDate) {
			continue
		}
		if err := inv.TransitionTo(StatusOverdue); err != nil {
			return nil, err
		}
		if !inv.LateFeeApplied {
			inv.LateFee = s.cfg.LateFee
			inv.LateFeeApplied = true
		}
		if err := s.invoices.Save(inv); err != nil {
			return nil, err
		}
		res.MarkedOverdue++
	}
	return res, nil
}

// RecordPayment applies a payment to an invoice. Replaying the same
// idempotency key returns the original payment without recording again.
// Partial payments are allowed; the invoice becomes paid only when the
// full total (rent + late fee) is covered.
func (s *Service) RecordPayment(invoiceID string, amount Money, idempotencyKey string) (*Payment, *Invoice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if idempotencyKey == "" {
		return nil, nil, fmt.Errorf("%w: idempotency_key is required", ErrInvalidInput)
	}
	if amount <= 0 {
		return nil, nil, fmt.Errorf("%w: amount must be positive", ErrInvalidInput)
	}

	// Idempotent replay: same key -> same result, no side effects.
	if prev, err := s.payments.GetByKey(idempotencyKey); err != nil {
		return nil, nil, err
	} else if prev != nil {
		inv, err := s.invoices.Get(prev.InvoiceID)
		if err != nil {
			return nil, nil, err
		}
		return prev, inv, nil
	}

	inv, err := s.invoices.Get(invoiceID)
	if err != nil {
		return nil, nil, err
	}
	if inv.Status != StatusIssued && inv.Status != StatusOverdue {
		return nil, nil, fmt.Errorf("%w: cannot pay invoice in state %q", ErrIllegalTransition, inv.Status)
	}
	if amount > inv.Outstanding() {
		return nil, nil, fmt.Errorf("%w: amount exceeds outstanding balance", ErrInvalidInput)
	}

	p := &Payment{
		ID:             uuid.NewString(),
		InvoiceID:      inv.ID,
		Amount:         amount,
		IdempotencyKey: idempotencyKey,
		ReceivedAt:     time.Now().UTC(),
	}
	inv.Paid += amount
	if inv.Outstanding() == 0 {
		if err := inv.TransitionTo(StatusPaid); err != nil {
			return nil, nil, err
		}
	}
	if err := s.payments.Save(p); err != nil {
		return nil, nil, err
	}
	if err := s.invoices.Save(inv); err != nil {
		return nil, nil, err
	}
	return p, inv, nil
}

func (s *Service) GetInvoice(id string) (*Invoice, error) { return s.invoices.Get(id) }

func (s *Service) ListInvoices(month string) ([]*Invoice, error) { return s.invoices.List(month) }
