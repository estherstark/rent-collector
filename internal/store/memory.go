// Package store provides in-memory repositories behind the billing
// interfaces. Postgres is next: the same interfaces get a pgx-backed
// implementation where unique constraints on (lease_id, month) and
// (idempotency_key) enforce idempotency at the DB level.
package store

import (
	"sort"
	"sync"

	"github.com/estherstark/rent-collector/internal/billing"
)

// Memory implements billing.LeaseRepo, billing.InvoiceRepo and
// billing.PaymentRepo. Values are copied on the way in and out so callers
// cannot mutate stored state without going through Save.
type Memory struct {
	mu       sync.RWMutex
	leases   map[string]billing.Lease
	invoices map[string]billing.Invoice
	payments map[string]billing.Payment // keyed by idempotency key
}

func NewMemory() *Memory {
	return &Memory{
		leases:   map[string]billing.Lease{},
		invoices: map[string]billing.Invoice{},
		payments: map[string]billing.Payment{},
	}
}

// --- billing.LeaseRepo ---

func (m *Memory) Save(l *billing.Lease) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.leases[l.ID] = *l
	return nil
}

func (m *Memory) Get(id string) (*billing.Lease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	l, ok := m.leases[id]
	if !ok {
		return nil, billing.ErrNotFound
	}
	return &l, nil
}

func (m *Memory) List() ([]*billing.Lease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*billing.Lease, 0, len(m.leases))
	for _, l := range m.leases {
		l := l
		out = append(out, &l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *Memory) ListActive() ([]*billing.Lease, error) {
	all, _ := m.List()
	out := all[:0]
	for _, l := range all {
		if l.Active {
			out = append(out, l)
		}
	}
	return out, nil
}

// --- billing.InvoiceRepo ---

// Invoices returns a facade so one Memory can serve all three interfaces
// without method-name collisions.
func (m *Memory) Invoices() *InvoiceStore { return &InvoiceStore{m} }
func (m *Memory) Payments() *PaymentStore { return &PaymentStore{m} }

type InvoiceStore struct{ m *Memory }

func (s *InvoiceStore) Save(inv *billing.Invoice) error {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	s.m.invoices[inv.ID] = *inv
	return nil
}

func (s *InvoiceStore) Get(id string) (*billing.Invoice, error) {
	s.m.mu.RLock()
	defer s.m.mu.RUnlock()
	inv, ok := s.m.invoices[id]
	if !ok {
		return nil, billing.ErrNotFound
	}
	return &inv, nil
}

// GetByLeaseMonth returns (nil, nil) when absent — this lookup is the
// in-memory stand-in for a UNIQUE (lease_id, month) constraint.
func (s *InvoiceStore) GetByLeaseMonth(leaseID, month string) (*billing.Invoice, error) {
	s.m.mu.RLock()
	defer s.m.mu.RUnlock()
	for _, inv := range s.m.invoices {
		if inv.LeaseID == leaseID && inv.Month == month {
			inv := inv
			return &inv, nil
		}
	}
	return nil, nil
}

func (s *InvoiceStore) ListByStatus(status billing.InvoiceStatus) ([]*billing.Invoice, error) {
	s.m.mu.RLock()
	defer s.m.mu.RUnlock()
	var out []*billing.Invoice
	for _, inv := range s.m.invoices {
		if inv.Status == status {
			inv := inv
			out = append(out, &inv)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// List returns all invoices, filtered by month when month != "".
func (s *InvoiceStore) List(month string) ([]*billing.Invoice, error) {
	s.m.mu.RLock()
	defer s.m.mu.RUnlock()
	var out []*billing.Invoice
	for _, inv := range s.m.invoices {
		if month == "" || inv.Month == month {
			inv := inv
			out = append(out, &inv)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// --- billing.PaymentRepo ---

type PaymentStore struct{ m *Memory }

func (s *PaymentStore) Save(p *billing.Payment) error {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	s.m.payments[p.IdempotencyKey] = *p
	return nil
}

// GetByKey returns (nil, nil) when absent — the stand-in for a UNIQUE
// (idempotency_key) constraint in Postgres.
func (s *PaymentStore) GetByKey(key string) (*billing.Payment, error) {
	s.m.mu.RLock()
	defer s.m.mu.RUnlock()
	p, ok := s.m.payments[key]
	if !ok {
		return nil, nil
	}
	return &p, nil
}
