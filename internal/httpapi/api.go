// Package httpapi is the thin HTTP layer over the billing service.
// All business rules live in internal/billing; handlers only translate
// JSON <-> domain and map typed errors to status codes.
package httpapi

import (
	"errors"
	"math"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/estherstark/rent-collector/internal/billing"
)

type API struct {
	svc *billing.Service
}

func New(svc *billing.Service) *API { return &API{svc: svc} }

func (a *API) Register(app *fiber.App) {
	a.registerPlayground(app)
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})
	app.Post("/leases", a.createLease)
	app.Get("/leases", a.listLeases)
	app.Post("/invoices/generate", a.generateInvoices)
	app.Post("/invoices/late-fees", a.applyLateFees)
	app.Get("/invoices", a.listInvoices)
	app.Get("/invoices/:id", a.getInvoice)
	app.Post("/payments", a.recordPayment)
}

// thbToSatang converts a THB amount from JSON into satang, rounding to the
// nearest satang once at the boundary; everything past this point is int64.
func thbToSatang(thb float64) billing.Money {
	return billing.Money(int64(math.Round(thb * 100)))
}

func fail(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, billing.ErrNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, billing.ErrIllegalTransition):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, billing.ErrInvalidInput):
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}

func (a *API) createLease(c *fiber.Ctx) error {
	var req struct {
		Tenant   string  `json:"tenant"`
		Property string  `json:"property"`
		RentTHB  float64 `json:"rent_thb"`
		DueDay   int     `json:"due_day"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fail(c, errors.Join(billing.ErrInvalidInput, err))
	}
	l, err := a.svc.CreateLease(req.Tenant, req.Property, thbToSatang(req.RentTHB), req.DueDay)
	if err != nil {
		return fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(l)
}

func (a *API) listLeases(c *fiber.Ctx) error {
	leases, err := a.svc.ListLeases()
	if err != nil {
		return fail(c, err)
	}
	if leases == nil {
		leases = []*billing.Lease{} // JSON [] instead of null for empty lists
	}
	return c.JSON(leases)
}

func (a *API) generateInvoices(c *fiber.Ctx) error {
	var req struct {
		Month string `json:"month"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fail(c, errors.Join(billing.ErrInvalidInput, err))
	}
	res, err := a.svc.GenerateInvoices(req.Month)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(res)
}

func (a *API) applyLateFees(c *fiber.Ctx) error {
	var req struct {
		AsOf string `json:"as_of"` // "2026-08-10"; empty = now
	}
	if err := c.BodyParser(&req); err != nil {
		return fail(c, errors.Join(billing.ErrInvalidInput, err))
	}
	asOf := time.Now().UTC()
	if req.AsOf != "" {
		t, err := time.Parse("2006-01-02", req.AsOf)
		if err != nil {
			return fail(c, errors.Join(billing.ErrInvalidInput, err))
		}
		asOf = t
	}
	res, err := a.svc.ApplyLateFees(asOf)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(res)
}

func (a *API) listInvoices(c *fiber.Ctx) error {
	invoices, err := a.svc.ListInvoices(c.Query("month"))
	if err != nil {
		return fail(c, err)
	}
	if invoices == nil {
		invoices = []*billing.Invoice{} // JSON [] instead of null for empty lists
	}
	return c.JSON(invoices)
}

func (a *API) getInvoice(c *fiber.Ctx) error {
	inv, err := a.svc.GetInvoice(c.Params("id"))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(inv)
}

func (a *API) recordPayment(c *fiber.Ctx) error {
	var req struct {
		InvoiceID      string  `json:"invoice_id"`
		AmountTHB      float64 `json:"amount_thb"`
		IdempotencyKey string  `json:"idempotency_key"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fail(c, errors.Join(billing.ErrInvalidInput, err))
	}
	p, inv, err := a.svc.RecordPayment(req.InvoiceID, thbToSatang(req.AmountTHB), req.IdempotencyKey)
	if err != nil {
		return fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"payment": p, "invoice": inv})
}
