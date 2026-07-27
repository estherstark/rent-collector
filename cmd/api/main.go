package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/estherstark/rent-collector/internal/billing"
	"github.com/estherstark/rent-collector/internal/httpapi"
	"github.com/estherstark/rent-collector/internal/store"
)

func main() {
	mem := store.NewMemory()
	svc := billing.NewService(billing.DefaultConfig(), mem, mem.Invoices(), mem.Payments())

	app := fiber.New(fiber.Config{AppName: "rent-collector"})
	app.Use(recover.New(), logger.New())
	httpapi.New(svc).Register(app)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Fatal(app.Listen(":" + port))
}
