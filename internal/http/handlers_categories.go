package http

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	db "one-more-mile/server/internal/sqlc"
)

type categoryResponse struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type createCategoryRequest struct {
	Name string `json:"name" validate:"required"`
}

type updateCategoryRequest struct {
	Name string `json:"name" validate:"required"`
}

func (h *Handler) ListCategories(c *fiber.Ctx) error {
	ctx, cancel := h.requestContext()
	defer cancel()

	categories, err := h.db.ListCategories(ctx)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load categories")
	}

	response := make([]categoryResponse, 0, len(categories))
	for _, category := range categories {
		response = append(response, mapCategory(category))
	}

	return c.JSON(response)
}

func (h *Handler) CreateCategory(c *fiber.Ctx) error {
	var req createCategoryRequest
	if err := h.parseBody(c, &req); err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid request")
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return h.respondError(c, fiber.StatusBadRequest, "invalid category")
	}

	ctx, cancel := h.requestContext()
	defer cancel()

	category, err := h.db.CreateCategory(ctx, name)
	if err != nil {
		if isUniqueViolation(err) {
			return h.respondError(c, fiber.StatusConflict, "category already exists")
		}
		return h.respondError(c, fiber.StatusInternalServerError, "failed to create category")
	}

	return c.JSON(mapCategory(category))
}

func (h *Handler) UpdateCategory(c *fiber.Ctx) error {
	oldName := strings.TrimSpace(c.Params("name"))
	if oldName == "" {
		return h.respondError(c, fiber.StatusBadRequest, "invalid category")
	}

	var req updateCategoryRequest
	if err := h.parseBody(c, &req); err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid request")
	}

	newName := strings.TrimSpace(req.Name)
	if newName == "" {
		return h.respondError(c, fiber.StatusBadRequest, "invalid category")
	}

	ctx, cancel := h.requestContext()
	defer cancel()

	category, err := h.db.UpdateCategory(ctx, db.UpdateCategoryParams{
		Name:   oldName,
		Name_2: newName,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return h.respondError(c, fiber.StatusNotFound, "category not found")
		}
		if isUniqueViolation(err) {
			return h.respondError(c, fiber.StatusConflict, "category already exists")
		}
		return h.respondError(c, fiber.StatusInternalServerError, "failed to update category")
	}

	return c.JSON(mapCategory(category))
}

func (h *Handler) DeleteCategory(c *fiber.Ctx) error {
	name := strings.TrimSpace(c.Params("name"))
	if name == "" {
		return h.respondError(c, fiber.StatusBadRequest, "invalid category")
	}

	ctx, cancel := h.requestContext()
	defer cancel()

	category, err := h.db.DeleteCategory(ctx, name)
	if err != nil {
		if err == pgx.ErrNoRows {
			return h.respondError(c, fiber.StatusNotFound, "category not found")
		}
		if isForeignKeyViolation(err) {
			return h.respondError(c, fiber.StatusConflict, "category is in use")
		}
		return h.respondError(c, fiber.StatusInternalServerError, "failed to delete category")
	}

	return c.JSON(mapCategory(category))
}

func mapCategory(category db.MerchantCategory) categoryResponse {
	return categoryResponse{
		Name:      category.Name,
		CreatedAt: fromPgTimestamptz(category.CreatedAt),
	}
}

func isForeignKeyViolation(err error) bool {
	pgErr, ok := err.(*pgconn.PgError)
	if !ok {
		return false
	}
	return pgErr.Code == "23503"
}
