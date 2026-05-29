package http

import (
	"crypto/subtle"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"one-more-mile/server/internal/http/middleware"
	db "one-more-mile/server/internal/sqlc"
	"one-more-mile/server/internal/util"
)

type otpRequest struct {
	Phone string `json:"phone" validate:"required"`
}

type otpResponse struct {
	OTPID     uuid.UUID `json:"otp_id"`
	ExpiresAt time.Time `json:"expires_at"`
	DebugCode string    `json:"debug_code,omitempty"`
}

func (h *Handler) SendOTP(c *fiber.Ctx) error {
	var req otpRequest
	if err := h.parseBody(c, &req); err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid request")
	}

	code, err := util.RandomDigits(6)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to generate otp")
	}

	otpID := uuid.New()
	expiresAt := time.Now().Add(5 * time.Minute)
	codeHash := util.HashOTP(req.Phone, code)

	ctx, cancel := h.requestContext()
	defer cancel()

	_, err = h.db.CreateOTP(ctx, db.CreateOTPParams{
		ID:        otpID,
		Phone:     req.Phone,
		CodeHash:  codeHash,
		ExpiresAt: toPgTimestamptz(expiresAt),
	})
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to create otp")
	}

	resp := otpResponse{
		OTPID:     otpID,
		ExpiresAt: expiresAt,
	}
	if h.cfg.Env == "dev" {
		resp.DebugCode = code
	}

	return c.JSON(resp)
}

type verifyOTPRequest struct {
	OTPID uuid.UUID `json:"otp_id" validate:"required"`
	Phone string    `json:"phone" validate:"required"`
	Code  string    `json:"code" validate:"required"`
}

type verifyOTPResponse struct {
	Token string       `json:"token"`
	User  userResponse `json:"user"`
}

func (h *Handler) VerifyOTP(c *fiber.Ctx) error {
	var req verifyOTPRequest
	if err := h.parseBody(c, &req); err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid request")
	}

	ctx, cancel := h.requestContext()
	defer cancel()

	otp, err := h.db.GetOTPForVerify(ctx, db.GetOTPForVerifyParams{
		ID:    req.OTPID,
		Phone: req.Phone,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return h.respondError(c, fiber.StatusUnauthorized, "invalid otp")
		}
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load otp")
	}

	expiry := fromPgTimestamptz(otp.ExpiresAt)
	if otp.UsedAt.Valid || !otp.ExpiresAt.Valid || time.Now().After(expiry) {
		return h.respondError(c, fiber.StatusUnauthorized, "otp expired")
	}

	expected := util.HashOTP(req.Phone, req.Code)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(otp.CodeHash)) != 1 {
		return h.respondError(c, fiber.StatusUnauthorized, "invalid otp")
	}

	if err := h.db.MarkOTPUsed(ctx, otp.ID); err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to update otp")
	}

	user, err := h.db.GetUserByPhone(ctx, req.Phone)
	if err != nil {
		if err == pgx.ErrNoRows {
			user, err = h.db.CreateUser(ctx, db.CreateUserParams{
				ID:    uuid.New(),
				Phone: req.Phone,
				Role:  "user",
			})
			if err != nil {
				return h.respondError(c, fiber.StatusInternalServerError, "failed to create user")
			}
		} else {
			return h.respondError(c, fiber.StatusInternalServerError, "failed to load user")
		}
	}

	token, err := middleware.GenerateToken(h.cfg, user.ID, user.Role)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to issue token")
	}

	return c.JSON(verifyOTPResponse{
		Token: token,
		User:  mapUser(user),
	})
}
