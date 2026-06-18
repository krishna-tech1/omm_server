package http

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"bytes"
	"encoding/base64"
	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/code128"
	"image/png"

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
		Role:      string(user.Role),
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

type couponResponse struct {
	ID               uuid.UUID  `json:"id"`
	Code             string     `json:"code"`
	Status           string     `json:"status"`
	ChallengeID      uuid.UUID  `json:"challenge_id"`
	SessionID        uuid.UUID  `json:"session_id"`
	IssuedAt         time.Time  `json:"issued_at"`
	RedeemedAt       *time.Time `json:"redeemed_at,omitempty"`
	RedeemedEmployee *uuid.UUID `json:"redeemed_by_employee_id,omitempty"`
	BarcodeBase64    string     `json:"barcode_base64,omitempty"`
}

func (h *Handler) ListUserCoupons(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}

	ctx, cancel := h.requestContext()
	defer cancel()

	coupons, err := h.db.ListCouponsByUser(ctx, claims.UserID)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load coupons")
	}

	response := make([]couponResponse, 0, len(coupons))
	for _, coupon := range coupons {
		response = append(response, mapCoupon(coupon))
	}

	return c.JSON(response)
}

func mapCoupon(coupon db.Coupon) couponResponse {
	resp := couponResponse{
		ID:               coupon.ID,
		Code:             coupon.Code,
		Status:           coupon.Status,
		ChallengeID:      coupon.ChallengeID,
		SessionID:        coupon.SessionID,
		IssuedAt:         fromPgTimestamptz(coupon.IssuedAt),
		RedeemedAt:       timePtrFromTimestamptz(coupon.RedeemedAt),
		RedeemedEmployee: uuidPtrFromUUID(coupon.RedeemedByEmployeeID),
	}

	if coupon.Status == "active" {
		if bc, err := code128.Encode(coupon.Code); err == nil {
			if scaled, err := barcode.Scale(bc, 300, 100); err == nil {
				var buf bytes.Buffer
				if err := png.Encode(&buf, scaled); err == nil {
					resp.BarcodeBase64 = base64.StdEncoding.EncodeToString(buf.Bytes())
				}
			}
		}
	}

	return resp
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
