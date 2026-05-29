package http

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	db "one-more-mile/server/internal/sqlc"
	"one-more-mile/server/internal/util"
)

type registerMerchantRequest struct {
	Name        string  `json:"name" validate:"required"`
	Category    string  `json:"category"`
	AddressLat  float64 `json:"address_lat"`
	AddressLng  float64 `json:"address_lng"`
	LogoURL     string  `json:"logo_url"`
	Description string  `json:"description"`
}

type merchantResponse struct {
	ID          uuid.UUID `json:"id"`
	OwnerUserID uuid.UUID `json:"owner_user_id"`
	Name        string    `json:"name"`
	Category    string    `json:"category"`
	AddressLat  float64   `json:"address_lat"`
	AddressLng  float64   `json:"address_lng"`
	LogoURL     string    `json:"logo_url"`
	Description string    `json:"description"`
}

type merchantDashboardResponse struct {
	TotalChallenges  int64 `json:"total_challenges"`
	TotalRedemptions int64 `json:"total_redemptions"`
	ActiveCustomers  int64 `json:"active_customers"`
}

type employeeResponse struct {
	ID         uuid.UUID `json:"id"`
	MerchantID uuid.UUID `json:"merchant_id"`
	Name       string    `json:"name"`
	Phone      string    `json:"phone"`
	Code       string    `json:"code"`
	Status     string    `json:"status"`
}

type createEmployeeRequest struct {
	Name  string `json:"name" validate:"required"`
	Phone string `json:"phone"`
}

func (h *Handler) RegisterMerchant(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}

	var req registerMerchantRequest
	if err := h.parseBody(c, &req); err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid request")
	}

	ctx, cancel := h.requestContext()
	defer cancel()

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to start transaction")
	}
	defer tx.Rollback(ctx)

	queries := h.db.WithTx(tx)

	merchant, err := queries.CreateMerchant(ctx, db.CreateMerchantParams{
		ID:          uuid.New(),
		OwnerUserID: claims.UserID,
		Name:        req.Name,
		Category:    req.Category,
		AddressLat:  req.AddressLat,
		AddressLng:  req.AddressLng,
		LogoUrl:     req.LogoURL,
		Description: req.Description,
	})
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to create merchant")
	}

	if err := queries.UpdateUserRole(ctx, db.UpdateUserRoleParams{
		ID:   claims.UserID,
		Role: "merchant",
	}); err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to update user role")
	}

	if err := tx.Commit(ctx); err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to commit merchant")
	}

	return c.JSON(mapMerchant(merchant))
}

func (h *Handler) MerchantDashboard(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
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

	stats, err := h.db.MerchantDashboardStats(ctx, merchant.ID)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load stats")
	}

	return c.JSON(merchantDashboardResponse{
		TotalChallenges:  stats.TotalChallenges,
		TotalRedemptions: stats.TotalRedemptions,
		ActiveCustomers:  stats.ActiveCustomers,
	})
}

func (h *Handler) ListEmployees(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
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

	employees, err := h.db.ListEmployeesByMerchant(ctx, merchant.ID)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load employees")
	}

	response := make([]employeeResponse, 0, len(employees))
	for _, employee := range employees {
		response = append(response, mapEmployee(employee))
	}

	return c.JSON(response)
}

func (h *Handler) CreateEmployee(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}

	var req createEmployeeRequest
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

	employee, err := h.createEmployeeWithUniqueCode(ctx, merchant.ID, req)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(mapEmployee(employee))
}

func (h *Handler) createEmployeeWithUniqueCode(ctx context.Context, merchantID uuid.UUID, req createEmployeeRequest) (db.Employee, error) {
	var lastErr error
	for i := 0; i < 10; i++ {
		code, err := util.RandomDigits(3)
		if err != nil {
			return db.Employee{}, err
		}

		employee, err := h.db.CreateEmployee(ctx, db.CreateEmployeeParams{
			ID:         uuid.New(),
			MerchantID: merchantID,
			Name:       req.Name,
			Phone:      req.Phone,
			Code:       code,
			Status:     "active",
		})
		if err == nil {
			return employee, nil
		}

		if isUniqueViolation(err) {
			lastErr = err
			continue
		}

		return db.Employee{}, err
	}

	if lastErr != nil {
		return db.Employee{}, errors.New("failed to allocate employee code")
	}
	return db.Employee{}, errors.New("failed to create employee")
}

func isUniqueViolation(err error) bool {
	pgErr, ok := err.(*pgconn.PgError)
	if !ok {
		return false
	}
	return pgErr.Code == "23505"
}

func mapMerchant(merchant db.Merchant) merchantResponse {
	return merchantResponse{
		ID:          merchant.ID,
		OwnerUserID: merchant.OwnerUserID,
		Name:        merchant.Name,
		Category:    merchant.Category,
		AddressLat:  merchant.AddressLat,
		AddressLng:  merchant.AddressLng,
		LogoURL:     merchant.LogoUrl,
		Description: merchant.Description,
	}
}

func mapEmployee(employee db.Employee) employeeResponse {
	return employeeResponse{
		ID:         employee.ID,
		MerchantID: employee.MerchantID,
		Name:       employee.Name,
		Phone:      employee.Phone,
		Code:       employee.Code,
		Status:     employee.Status,
	}
}
