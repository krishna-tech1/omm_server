package http

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	db "one-more-mile/server/internal/sqlc"
)

type redeemCouponRequest struct {
	CouponCode string `json:"coupon_code" validate:"required"`
}

type redeemCouponResponse struct {
	CouponID   uuid.UUID `json:"coupon_id"`
	EmployeeID uuid.UUID `json:"employee_id"`
	RedeemedAt string    `json:"redeemed_at"`
	CouponCode string    `json:"coupon_code"`
}

func (h *Handler) RedeemCoupon(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}

	var req redeemCouponRequest
	if err := h.parseBody(c, &req); err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid request")
	}

	ctx, cancel := h.requestContext()
	defer cancel()

	employee, err := h.db.GetEmployeeByUserID(ctx, claims.UserID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return h.respondError(c, fiber.StatusForbidden, "employee not found")
		}
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load employee")
	}

	if employee.Status != "active" {
		return h.respondError(c, fiber.StatusForbidden, "employee inactive")
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to start transaction")
	}
	defer tx.Rollback(ctx)

	queries := h.db.WithTx(tx)

	coupon, err := queries.GetCouponForUpdate(ctx, req.CouponCode)
	if err != nil {
		if err == pgx.ErrNoRows {
			return h.respondError(c, fiber.StatusNotFound, "coupon not found")
		}
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load coupon")
	}

	if coupon.Status != "active" {
		return h.respondError(c, fiber.StatusBadRequest, "coupon not active")
	}

	challenge, err := queries.GetChallengeByID(ctx, coupon.ChallengeID)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load challenge")
	}

	if challenge.MerchantID != employee.MerchantID {
		return h.respondError(c, fiber.StatusForbidden, "coupon does not belong to merchant")
	}

	if err := queries.MarkCouponRedeemed(ctx, db.MarkCouponRedeemedParams{
		ID:                   coupon.ID,
		RedeemedByEmployeeID: toPgUUID(employee.ID),
	}); err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to redeem coupon")
	}

	redemption, err := queries.CreateCouponRedemption(ctx, db.CreateCouponRedemptionParams{
		ID:         uuid.New(),
		CouponID:   coupon.ID,
		EmployeeID: employee.ID,
	})
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to record redemption")
	}

	if err := tx.Commit(ctx); err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to commit redemption")
	}

	return c.JSON(redeemCouponResponse{
		CouponID:   coupon.ID,
		EmployeeID: employee.ID,
		RedeemedAt: fromPgTimestamptz(redemption.RedeemedAt).Format(time.RFC3339),
		CouponCode: coupon.Code,
	})
}
