package http

import (
	"context"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"one-more-mile/server/internal/config"
	"one-more-mile/server/internal/http/middleware"
	db "one-more-mile/server/internal/sqlc"
)

type Handler struct {
	cfg      config.Config
	db       *db.Queries
	pool     *pgxpool.Pool
	validate *validator.Validate
}

func NewHandler(cfg config.Config, queries *db.Queries, pool *pgxpool.Pool) *Handler {
	return &Handler{
		cfg:      cfg,
		db:       queries,
		pool:     pool,
		validate: validator.New(),
	}
}

type errorResponse struct {
	Error string `json:"error"`
}

func (h *Handler) respondError(c *fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(errorResponse{Error: message})
}

func (h *Handler) parseBody(c *fiber.Ctx, dest interface{}) error {
	if err := c.BodyParser(dest); err != nil {
		return err
	}
	return h.validate.Struct(dest)
}

func (h *Handler) requestContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func getClaims(c *fiber.Ctx) (middleware.Claims, bool) {
	return middleware.GetClaims(c)
}

func toPgTimestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func fromPgTimestamptz(value pgtype.Timestamptz) time.Time {
	if value.Valid {
		return value.Time
	}
	return time.Time{}
}

func toPgUUID(value uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: value, Valid: true}
}
