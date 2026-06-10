package http

import (
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"one-more-mile/server/internal/config"
	"one-more-mile/server/internal/http/middleware"
	db "one-more-mile/server/internal/sqlc"
)

func NewServer(cfg config.Config, queries *db.Queries, pool *pgxpool.Pool, redisClient *redis.Client) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:      "omm-server",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	})

	app.Use(recover.New())
	if cfg.Env == "dev" {
		app.Use(logger.New())
	}

	handler := NewHandler(cfg, queries, pool, redisClient)

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	api := app.Group("/api")
	api.Get("/sessions/:id/stream", handler.AuthWebSocket, websocket.New(handler.StreamSession, websocket.Config{
		HandshakeTimeout: 5 * time.Second,
		ReadBufferSize:   2048,
		WriteBufferSize:  2048,
	}))

	auth := api.Group("/auth")
	auth.Post("/otp", handler.SendOTP)
	auth.Post("/verify", handler.VerifyOTP)

	protected := api.Group("", middleware.Auth(cfg))

	// Notifications (SSE)
	notifications := protected.Group("/notifications")
	notifications.Get("/stream", handler.StreamNotifications)

	// Payments
	payments := protected.Group("/payments")
	payments.Post("/razorpay/order", handler.CreateRazorpayOrder)
	api.Post("/payments/razorpay/webhook", handler.RazorpayWebhook) // Public webhook

	users := protected.Group("/users")
	users.Post("/profile", handler.UpdateProfile)
	users.Get("/coupons", handler.ListUserCoupons)

	uploads := protected.Group("/uploads")
	uploads.Post("/presign", handler.PresignUpload)

	sessions := protected.Group("/sessions")
	sessions.Post("/start", handler.StartSession)
	sessions.Post("/stop", handler.StopSession)

	challenges := protected.Group("/challenges")
	challenges.Get("", handler.ListChallenges)
	challenges.Post("/:id/register", handler.RegisterChallenge)
	challenges.Post("/:id", middleware.RequireRole("merchant", "admin"), handler.UpdateChallenge)
	challenges.Post("", middleware.RequireRole("merchant", "admin"), handler.CreateChallenge)

	merchants := protected.Group("/merchants")
	merchants.Get("", middleware.RequireRole("merchant"), handler.GetMerchantProfile)
	merchants.Post("", middleware.RequireRole("merchant"), handler.UpdateMerchantProfile)
	merchants.Get("/all", middleware.RequireRole("admin"), handler.ListAllMerchants)
	merchants.Get("/nearby", handler.ListNearbyMerchants)
	merchants.Post("/register", handler.RegisterMerchant)
	merchants.Get("/dashboard", middleware.RequireRole("merchant", "admin"), handler.MerchantDashboard)
	merchants.Get("/challenges", middleware.RequireRole("merchant", "admin"), handler.ListMerchantChallenges)
	merchants.Get("/employees", middleware.RequireRole("merchant", "admin"), handler.ListEmployees)
	merchants.Post("/employees", middleware.RequireRole("merchant", "admin"), handler.CreateEmployee)
	merchants.Patch("/employees/:id", middleware.RequireRole("merchant", "admin"), handler.UpdateEmployee)
	merchants.Delete("/employees/:id", middleware.RequireRole("merchant", "admin"), handler.DeleteEmployee)

	categories := protected.Group("/categories")
	categories.Get("", handler.ListCategories)
	categories.Post("", middleware.RequireRole("admin"), handler.CreateCategory)
	categories.Patch("/:name", middleware.RequireRole("admin"), handler.UpdateCategory)
	categories.Delete("/:name", middleware.RequireRole("admin"), handler.DeleteCategory)

	coupons := protected.Group("/coupons")
	coupons.Post("/redeem", middleware.RequireRole("employee"), handler.RedeemCoupon)

	admin := protected.Group("/admin", middleware.RequireRole("admin"))
	admin.Get("/stats", handler.AdminStats)
	admin.Post("/merchants/:id/ban", handler.BanMerchant)

	return app
}
