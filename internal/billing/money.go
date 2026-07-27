package billing

// Money is an amount in satang (1/100 THB), stored as int64.
// NEVER use float for money: binary floats cannot represent 0.1 THB
// exactly, and rounding errors compound across invoices.
type Money int64

// THB converts whole baht to Money (satang).
func THB(baht int64) Money { return Money(baht * 100) }

// Baht returns the whole-baht part (truncated) for display.
func (m Money) Baht() float64 { return float64(m) / 100 }
