package http

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "one-more-mile/server/internal/sqlc"
)

type userResponse struct {
	ID        uuid.UUID `json:"id"`
	Phone     string    `json:"phone"`
	Name      string    `json:"name"`
	AvatarURL string    `json:"avatar_url"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

func mapUser(user db.User) userResponse {
	return userResponse{
		ID:        user.ID,
		Phone:     user.Phone,
		Name:      user.Name,
		AvatarURL: user.AvatarUrl,
		Role:      user.Role,
		CreatedAt: fromPgTimestamptz(user.CreatedAt),
	}
}

type updateProfileRequest struct {
	Name      string `json:"name" validate:"required"`
	AvatarURL string `json:"avatar_url"`
}

func (h *Handler) UpdateProfile(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}

	var req updateProfileRequest
	if err := h.parseBody(c, &req); err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid request")
	}

	ctx, cancel := h.requestContext()
	defer cancel()

	user, err := h.db.UpdateUserProfile(ctx, db.UpdateUserProfileParams{
		ID:        claims.UserID,
		Name:      req.Name,
		AvatarUrl: req.AvatarURL,
	})
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to update profile")
	}

	return c.JSON(mapUser(user))
}

type voucherResponse struct {
	ID               uuid.UUID  `json:"id"`
	Code             string     `json:"code"`
	Status           string     `json:"status"`
	ChallengeID      uuid.UUID  `json:"challenge_id"`
	SessionID        uuid.UUID  `json:"session_id"`
	IssuedAt         time.Time  `json:"issued_at"`
	RedeemedAt       *time.Time `json:"redeemed_at,omitempty"`
	RedeemedEmployee *uuid.UUID `json:"redeemed_by_employee_id,omitempty"`
}

func (h *Handler) ListUserVouchers(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}

	ctx, cancel := h.requestContext()
	defer cancel()

	vouchers, err := h.db.ListVouchersByUser(ctx, claims.UserID)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load vouchers")
	}

	response := make([]voucherResponse, 0, len(vouchers))
	for _, voucher := range vouchers {
		response = append(response, mapVoucher(voucher))
	}

	return c.JSON(response)
}

func mapVoucher(voucher db.Voucher) voucherResponse {
	return voucherResponse{
		ID:               voucher.ID,
		Code:             voucher.Code,
		Status:           voucher.Status,
		ChallengeID:      voucher.ChallengeID,
		SessionID:        voucher.SessionID,
		IssuedAt:         fromPgTimestamptz(voucher.IssuedAt),
		RedeemedAt:       timePtrFromTimestamptz(voucher.RedeemedAt),
		RedeemedEmployee: uuidPtrFromUUID(voucher.RedeemedByEmployeeID),
	}
}

func timePtrFromTimestamptz(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func uuidPtrFromUUID(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	parsed, err := uuid.FromBytes(value.Bytes[:])
	if err != nil {
		return nil
	}
	return &parsed
}
