package http

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"one-more-mile/server/internal/http/middleware"
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
	UserID     uuid.UUID `json:"user_id"`
	Name       string    `json:"name"`
	Phone      string    `json:"phone"`
	Code       string    `json:"code"`
	Status     string    `json:"status"`
}

type createEmployeeRequest struct {
	Name  string `json:"name" validate:"required"`
	Phone string `json:"phone" validate:"required"`
}

type updateEmployeeRequest struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

const defaultCategoryName = "Uncategorized"

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

	category := strings.TrimSpace(req.Category)
	if category == "" {
		category = defaultCategoryName
	}
	if _, err := h.db.GetCategoryByName(ctx, category); err != nil {
		if err == pgx.ErrNoRows {
			return h.respondError(c, fiber.StatusBadRequest, "invalid category")
		}
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load category")
	}

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
		Category:    category,
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
		Role: db.UserRoleMerchant,
	}); err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to update user role")
	}

	if err := tx.Commit(ctx); err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to commit merchant")
	}

	// Generate new token with merchant role
	token, err := middleware.GenerateToken(h.cfg, claims.UserID, string(db.UserRoleMerchant))
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to generate token")
	}

	return c.JSON(fiber.Map{
		"merchant": mapMerchant(merchant),
		"token":    token,
	})
}

func (h *Handler) GetMerchantProfile(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}

	ctx, cancel := h.requestContext()
	defer cancel()

	merchant, err := h.db.GetMerchantByOwner(ctx, claims.UserID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return h.respondError(c, fiber.StatusNotFound, "merchant not found for this user")
		}
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load merchant")
	}

	return c.JSON(mapMerchant(merchant))
}

func (h *Handler) UpdateMerchantProfile(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}

	ctx, cancel := h.requestContext()
	defer cancel()

	var req registerMerchantRequest
	if err := h.parseBody(c, &req); err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid request")
	}

	merchant, err := h.db.GetMerchantByOwner(ctx, claims.UserID)
	if err != nil {
		return h.respondError(c, fiber.StatusNotFound, "merchant not found for this user")
	}

	category := strings.TrimSpace(req.Category)
	if category == "" {
		category = defaultCategoryName
	}
	if _, err := h.db.GetCategoryByName(ctx, category); err != nil {
		if err == pgx.ErrNoRows {
			return h.respondError(c, fiber.StatusBadRequest, "invalid category")
		}
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load category")
	}

	updatedMerchant, err := h.db.UpdateMerchantProfile(ctx, db.UpdateMerchantProfileParams{
		ID:          merchant.ID,
		Name:        req.Name,
		Category:    category,
		AddressLat:  req.AddressLat,
		AddressLng:  req.AddressLng,
		LogoUrl:     req.LogoURL,
		Description: req.Description,
	})
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to update merchant")
	}

	return c.JSON(mapMerchant(updatedMerchant))
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

	user, err := h.db.GetUserByPhone(ctx, req.Phone)
	if err != nil {
		if err == pgx.ErrNoRows {
			user, err = h.db.CreateUser(ctx, db.CreateUserParams{
				ID:    uuid.New(),
				Phone: req.Phone,
				Role:  db.UserRoleEmployee,
			})
			if err != nil {
				return h.respondError(c, fiber.StatusInternalServerError, "failed to create employee user")
			}
		} else {
			return h.respondError(c, fiber.StatusInternalServerError, "failed to load employee user")
		}
	}

	if user.Role != db.UserRoleEmployee {
		if err := h.db.UpdateUserRole(ctx, db.UpdateUserRoleParams{
			ID:   user.ID,
			Role: db.UserRoleEmployee,
		}); err != nil {
			return h.respondError(c, fiber.StatusInternalServerError, "failed to update employee role")
		}
		user.Role = db.UserRoleEmployee
	}

	if _, err := h.db.GetEmployeeByUserID(ctx, user.ID); err == nil {
		return h.respondError(c, fiber.StatusConflict, "employee already exists")
	} else if err != pgx.ErrNoRows {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load employee")
	}

	employee, err := h.createEmployeeWithUniqueCode(ctx, merchant.ID, user, req)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(mapEmployee(employee))
}

func (h *Handler) createEmployeeWithUniqueCode(ctx context.Context, merchantID uuid.UUID, user db.User, req createEmployeeRequest) (db.Employee, error) {
	var lastErr error
	for i := 0; i < 10; i++ {
		code, err := util.RandomDigits(3)
		if err != nil {
			return db.Employee{}, err
		}

		employee, err := h.db.CreateEmployee(ctx, db.CreateEmployeeParams{
			ID:         uuid.New(),
			MerchantID: merchantID,
			UserID:     user.ID,
			Name:       req.Name,
			Phone:      user.Phone,
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
		UserID:     employee.UserID,
		Name:       employee.Name,
		Phone:      employee.Phone,
		Code:       employee.Code,
		Status:     employee.Status,
	}
}

func (h *Handler) UpdateEmployee(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}

	employeeID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid employee id")
	}

	var req updateEmployeeRequest
	if err := h.parseBody(c, &req); err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid request")
	}

	if strings.TrimSpace(req.Name) == "" && strings.TrimSpace(req.Status) == "" {
		return h.respondError(c, fiber.StatusBadRequest, "missing updates")
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

	updated, err := h.db.UpdateEmployee(ctx, db.UpdateEmployeeParams{
		ID:         employeeID,
		MerchantID: merchant.ID,
		Column3:    req.Name,
		Column4:    req.Status,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return h.respondError(c, fiber.StatusNotFound, "employee not found")
		}
		return h.respondError(c, fiber.StatusInternalServerError, "failed to update employee")
	}

	return c.JSON(mapEmployee(updated))
}

func (h *Handler) DeleteEmployee(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}

	employeeID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid employee id")
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

	employee, err := h.db.DeactivateEmployee(ctx, db.DeactivateEmployeeParams{
		ID:         employeeID,
		MerchantID: merchant.ID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return h.respondError(c, fiber.StatusNotFound, "employee not found")
		}
		return h.respondError(c, fiber.StatusInternalServerError, "failed to deactivate employee")
	}

	return c.JSON(mapEmployee(employee))
}

func (h *Handler) ListNearbyMerchants(c *fiber.Ctx) error {
	ctx, cancel := h.requestContext()
	defer cancel()

	latStr := c.Query("lat")
	lngStr := c.Query("lng")

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid lat")
	}
	lng, err := strconv.ParseFloat(lngStr, 64)
	if err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid lng")
	}

	merchants, err := h.db.GetNearbyMerchants(ctx, db.GetNearbyMerchantsParams{
		Column1: lat,
		Column2: lng,
		Column3: 30.0,
	})
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to get nearby merchants")
	}

	// db returns []db.GetNearbyMerchantsRow
	// We might need to map them if we want a specific JSON shape,
	// but the row has all necessary json tags.
	return c.JSON(merchants)
}
