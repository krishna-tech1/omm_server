package http

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	db "one-more-mile/server/internal/sqlc"
)

type redeemVoucherRequest struct {
	VoucherCode  string `json:"voucher_code" validate:"required"`
	EmployeeCode string `json:"employee_code" validate:"required"`
}

type redeemVoucherResponse struct {
	VoucherID   uuid.UUID `json:"voucher_id"`
	EmployeeID  uuid.UUID `json:"employee_id"`
	RedeemedAt  string    `json:"redeemed_at"`
	VoucherCode string    `json:"voucher_code"`
}

func (h *Handler) RedeemVoucher(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}

	var req redeemVoucherRequest
	if err := h.parseBody(c, &req); err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid request")
	}

	ctx, cancel := h.requestContext()
	defer cancel()

	merchant, err := h.db.GetMerchantByOwner(ctx, claims.UserID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return h.respondError(c, fiber.StatusNotFound, "merchant not found")
		}
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load merchant")
	}

	employee, err := h.db.GetEmployeeByCode(ctx, db.GetEmployeeByCodeParams{
		MerchantID: merchant.ID,
		Code:       req.EmployeeCode,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return h.respondError(c, fiber.StatusNotFound, "employee not found")
		}
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load employee")
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to start transaction")
	}
	defer tx.Rollback(ctx)

	queries := h.db.WithTx(tx)

	voucher, err := queries.GetVoucherForUpdate(ctx, req.VoucherCode)
	if err != nil {
		if err == pgx.ErrNoRows {
			return h.respondError(c, fiber.StatusNotFound, "voucher not found")
		}
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load voucher")
	}

	if voucher.Status != "active" {
		return h.respondError(c, fiber.StatusBadRequest, "voucher not active")
	}

	challenge, err := queries.GetChallengeByID(ctx, voucher.ChallengeID)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load challenge")
	}

	if challenge.MerchantID != merchant.ID {
		return h.respondError(c, fiber.StatusForbidden, "voucher does not belong to merchant")
	}

	if err := queries.MarkVoucherRedeemed(ctx, db.MarkVoucherRedeemedParams{
		ID:                   voucher.ID,
		RedeemedByEmployeeID: toPgUUID(employee.ID),
	}); err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to redeem voucher")
	}

	redemption, err := queries.CreateVoucherRedemption(ctx, db.CreateVoucherRedemptionParams{
		ID:         uuid.New(),
		VoucherID:  voucher.ID,
		EmployeeID: employee.ID,
	})
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to record redemption")
	}

	if err := tx.Commit(ctx); err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to commit redemption")
	}

	return c.JSON(redeemVoucherResponse{
		VoucherID:   voucher.ID,
		EmployeeID:  employee.ID,
		RedeemedAt:  fromPgTimestamptz(redemption.RedeemedAt).Format(time.RFC3339),
		VoucherCode: voucher.Code,
	})
}
