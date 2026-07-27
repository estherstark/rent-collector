package billing_test

import (
	"errors"
	"testing"
	"time"

	"github.com/estherstark/rent-collector/internal/billing"
	"github.com/estherstark/rent-collector/internal/store"
)

func newService(t *testing.T) *billing.Service {
	t.Helper()
	mem := store.NewMemory()
	return billing.NewService(billing.DefaultConfig(), mem, mem.Invoices(), mem.Payments())
}

func mustLease(t *testing.T, s *billing.Service) *billing.Lease {
	t.Helper()
	l, err := s.CreateLease("Somchai", "Room 101", billing.THB(9000), 5)
	if err != nil {
		t.Fatalf("CreateLease: %v", err)
	}
	return l
}

func mustGenerate(t *testing.T, s *billing.Service, month string) *billing.GenerateResult {
	t.Helper()
	res, err := s.GenerateInvoices(month)
	if err != nil {
		t.Fatalf("GenerateInvoices(%s): %v", month, err)
	}
	return res
}

func soleInvoice(t *testing.T, s *billing.Service, month string) *billing.Invoice {
	t.Helper()
	invs, err := s.ListInvoices(month)
	if err != nil || len(invs) != 1 {
		t.Fatalf("want exactly 1 invoice for %s, got %d (err=%v)", month, len(invs), err)
	}
	return invs[0]
}

func TestGenerateInvoicesIdempotent(t *testing.T) {
	tests := []struct {
		name                         string
		runs                         int
		wantCreated, wantSkippedLast int
	}{
		{"first run creates", 1, 1, 0},
		{"second run skips", 2, 0, 1},
		{"third run still skips", 3, 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newService(t)
			mustLease(t, s)
			var last *billing.GenerateResult
			for i := 0; i < tt.runs; i++ {
				last = mustGenerate(t, s, "2026-08")
			}
			if last.Created != tt.wantCreated || last.Skipped != tt.wantSkippedLast {
				t.Errorf("run %d: created=%d skipped=%d, want %d/%d",
					tt.runs, last.Created, last.Skipped, tt.wantCreated, tt.wantSkippedLast)
			}
			if invs, _ := s.ListInvoices("2026-08"); len(invs) != 1 {
				t.Errorf("total invoices = %d, want 1", len(invs))
			}
		})
	}
}

func TestApplyLateFeesOnce(t *testing.T) {
	s := newService(t)
	mustLease(t, s) // due day 5
	mustGenerate(t, s, "2026-08")
	after := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		asOf        time.Time
		wantMarked  int
		wantLateFee billing.Money
		wantStatus  billing.InvoiceStatus
	}{
		{"before due date: nothing", time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), 0, 0, billing.StatusIssued},
		{"past due: fee applied", after, 1, billing.THB(500), billing.StatusOverdue},
		{"re-run: no double charge", after, 0, billing.THB(500), billing.StatusOverdue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := s.ApplyLateFees(tt.asOf)
			if err != nil {
				t.Fatalf("ApplyLateFees: %v", err)
			}
			if res.MarkedOverdue != tt.wantMarked {
				t.Errorf("marked=%d, want %d", res.MarkedOverdue, tt.wantMarked)
			}
			inv := soleInvoice(t, s, "2026-08")
			if inv.LateFee != tt.wantLateFee || inv.Status != tt.wantStatus {
				t.Errorf("late_fee=%d status=%s, want %d/%s", inv.LateFee, inv.Status, tt.wantLateFee, tt.wantStatus)
			}
		})
	}
}

func TestRecordPaymentIdempotencyKeyReplay(t *testing.T) {
	s := newService(t)
	mustLease(t, s)
	mustGenerate(t, s, "2026-08")
	inv := soleInvoice(t, s, "2026-08")

	p1, _, err := s.RecordPayment(inv.ID, billing.THB(3000), "key-1")
	if err != nil {
		t.Fatalf("first payment: %v", err)
	}
	p2, inv2, err := s.RecordPayment(inv.ID, billing.THB(3000), "key-1") // replay
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if p2.ID != p1.ID {
		t.Errorf("replay returned new payment %s, want original %s", p2.ID, p1.ID)
	}
	if inv2.Paid != billing.THB(3000) {
		t.Errorf("paid=%d after replay, want %d (no double-record)", inv2.Paid, billing.THB(3000))
	}
}

func TestIllegalTransitions(t *testing.T) {
	tests := []struct {
		name string
		from billing.InvoiceStatus
		to   billing.InvoiceStatus
		ok   bool
	}{
		{"draft to issued", billing.StatusDraft, billing.StatusIssued, true},
		{"draft to paid", billing.StatusDraft, billing.StatusPaid, false},
		{"issued to overdue", billing.StatusIssued, billing.StatusOverdue, true},
		{"issued to draft", billing.StatusIssued, billing.StatusDraft, false},
		{"overdue to paid", billing.StatusOverdue, billing.StatusPaid, true},
		{"paid to issued (terminal)", billing.StatusPaid, billing.StatusIssued, false},
		{"paid to overdue (terminal)", billing.StatusPaid, billing.StatusOverdue, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv := &billing.Invoice{Status: tt.from}
			err := inv.TransitionTo(tt.to)
			if tt.ok && err != nil {
				t.Errorf("want legal, got %v", err)
			}
			if !tt.ok && !errors.Is(err, billing.ErrIllegalTransition) {
				t.Errorf("want ErrIllegalTransition, got %v", err)
			}
		})
	}
}

func TestPartialPaymentFlow(t *testing.T) {
	s := newService(t)
	mustLease(t, s) // rent 9000 THB
	mustGenerate(t, s, "2026-08")
	invID := soleInvoice(t, s, "2026-08").ID

	steps := []struct {
		name       string
		amount     billing.Money
		key        string
		wantErr    error
		wantPaid   billing.Money
		wantStatus billing.InvoiceStatus
	}{
		{"partial keeps issued", billing.THB(4000), "p-1", nil, billing.THB(4000), billing.StatusIssued},
		{"overpay rejected", billing.THB(6000), "p-2", billing.ErrInvalidInput, billing.THB(4000), billing.StatusIssued},
		{"remainder pays in full", billing.THB(5000), "p-3", nil, billing.THB(9000), billing.StatusPaid},
		{"pay a paid invoice rejected", billing.THB(1), "p-4", billing.ErrIllegalTransition, billing.THB(9000), billing.StatusPaid},
	}
	for _, st := range steps {
		t.Run(st.name, func(t *testing.T) {
			_, _, err := s.RecordPayment(invID, st.amount, st.key)
			if !errors.Is(err, st.wantErr) {
				t.Fatalf("err=%v, want %v", err, st.wantErr)
			}
			inv, _ := s.GetInvoice(invID)
			if inv.Paid != st.wantPaid || inv.Status != st.wantStatus {
				t.Errorf("paid=%d status=%s, want %d/%s", inv.Paid, inv.Status, st.wantPaid, st.wantStatus)
			}
		})
	}
}
