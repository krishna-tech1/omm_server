package http

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	db "one-more-mile/server/internal/sqlc"
)

type challengeResponse struct {
	ID             uuid.UUID `json:"id"`
	MerchantID     uuid.UUID `json:"merchant_id"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	TargetMiles    float64   `json:"target_miles"`
	ExpiresAt      time.Time `json:"expires_at"`
	DurationDays   int32     `json:"duration_days"`
	Reward         string    `json:"reward"`
	RewardImageURL string    `json:"reward_image_url"`
	CreatedAt      time.Time `json:"created_at"`
}

type createChallengeRequest struct {
	Title          string    `json:"title" validate:"required"`
	Description    string    `json:"description"`
	TargetMiles    float64   `json:"target_miles" validate:"required"`
	ExpiresAt      time.Time `json:"expires_at" validate:"required"`
	DurationDays   int32     `json:"duration_days"`
	Reward         string    `json:"reward"`
	RewardImageURL string    `json:"reward_image_url"`
}

func (h *Handler) ListChallenges(c *fiber.Ctx) error {
	ctx, cancel := h.requestContext()
	defer cancel()

	challenges, err := h.db.ListActiveChallenges(ctx, toPgTimestamptz(time.Now()))
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load challenges")
	}

	response := make([]challengeResponse, 0, len(challenges))
	for _, challenge := range challenges {
		response = append(response, mapChallenge(challenge))
	}

	return c.JSON(response)
}

func (h *Handler) RegisterChallenge(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}

	challengeID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid challenge id")
	}

	ctx, cancel := h.requestContext()
	defer cancel()

	_, err = h.db.GetChallengeByID(ctx, challengeID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return h.respondError(c, fiber.StatusNotFound, "challenge not found")
		}
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load challenge")
	}

	user, err := h.db.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load user")
	}

	if !user.IsPremium {
		activeCount, err := h.db.CountActiveChallengeRegistrationsForUser(ctx, db.CountActiveChallengeRegistrationsForUserParams{
			UserID:    claims.UserID,
			ExpiresAt: toPgTimestamptz(time.Now()),
		})
		if err != nil {
			return h.respondError(c, fiber.StatusInternalServerError, "failed to count active challenges")
		}
		if activeCount >= 2 {
			return h.respondError(c, fiber.StatusForbidden, "non-premium users can only have up to 2 active challenges")
		}
	}

	registration, err := h.db.RegisterChallenge(ctx, db.RegisterChallengeParams{
		ID:          uuid.New(),
		ChallengeID: challengeID,
		UserID:      claims.UserID,
	})
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to register challenge")
	}

	registeredAt := fromPgTimestamptz(registration.RegisteredAt)
	return c.JSON(fiber.Map{
		"id":               registration.ID,
		"challenge_id":     registration.ChallengeID,
		"user_id":          registration.UserID,
		"registered_at":    registeredAt,
		"status":           registration.Status,
		"distance_covered": registration.DistanceCovered,
	})
}

func (h *Handler) ListUserChallengeRegistrations(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}

	ctx, cancel := h.requestContext()
	defer cancel()

	registrations, err := h.db.GetChallengeRegistrationsForUser(ctx, claims.UserID)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load registrations")
	}

	response := make([]fiber.Map, 0, len(registrations))
	for _, reg := range registrations {
		response = append(response, fiber.Map{
			"id":               reg.ID,
			"challenge_id":     reg.ChallengeID,
			"user_id":          reg.UserID,
			"registered_at":    fromPgTimestamptz(reg.RegisteredAt),
			"status":           reg.Status,
			"distance_covered": reg.DistanceCovered,
		})
	}

	return c.JSON(response)
}

func (h *Handler) CreateChallenge(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}

	var req createChallengeRequest
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

	challenge, err := h.db.CreateChallenge(ctx, db.CreateChallengeParams{
		ID:             uuid.New(),
		MerchantID:     merchant.ID,
		Title:          req.Title,
		Description:    req.Description,
		TargetMiles:    req.TargetMiles,
		ExpiresAt:      toPgTimestamptz(req.ExpiresAt),
		DurationDays:   req.DurationDays,
		Reward:         req.Reward,
		RewardImageUrl: req.RewardImageURL,
	})
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to create challenge")
	}

	return c.JSON(mapChallenge(challenge))
}

func (h *Handler) UpdateChallenge(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}

	challengeID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid challenge id")
	}

	var req createChallengeRequest
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

	challenge, err := h.db.UpdateChallenge(ctx, db.UpdateChallengeParams{
		ID:             challengeID,
		Title:          req.Title,
		Description:    req.Description,
		TargetMiles:    req.TargetMiles,
		ExpiresAt:      toPgTimestamptz(req.ExpiresAt),
		DurationDays:   req.DurationDays,
		Reward:         req.Reward,
		RewardImageUrl: req.RewardImageURL,
		MerchantID:     merchant.ID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return h.respondError(c, fiber.StatusNotFound, "challenge not found or unauthorized")
		}
		return h.respondError(c, fiber.StatusInternalServerError, "failed to update challenge")
	}

	return c.JSON(mapChallenge(challenge))
}

func (h *Handler) ListMerchantChallenges(c *fiber.Ctx) error {
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

	challenges, err := h.db.ListMerchantChallengesWithCounts(ctx, merchant.ID)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load challenges")
	}

	response := make([]fiber.Map, 0, len(challenges))
	for _, challenge := range challenges {
		response = append(response, fiber.Map{
			"id":               challenge.ID,
			"merchant_id":      challenge.MerchantID,
			"title":            challenge.Title,
			"description":      challenge.Description,
			"target_miles":     challenge.TargetMiles,
			"expires_at":       fromPgTimestamptz(challenge.ExpiresAt),
			"duration_days":    challenge.DurationDays,
			"reward":           challenge.Reward,
			"reward_image_url": challenge.RewardImageUrl,
			"created_at":       fromPgTimestamptz(challenge.CreatedAt),
			"participants":     challenge.Participants,
		})
	}

	return c.JSON(response)
}

func mapChallenge(challenge db.Challenge) challengeResponse {
	return challengeResponse{
		ID:             challenge.ID,
		MerchantID:     challenge.MerchantID,
		Title:          challenge.Title,
		Description:    challenge.Description,
		TargetMiles:    challenge.TargetMiles,
		ExpiresAt:      fromPgTimestamptz(challenge.ExpiresAt),
		DurationDays:   challenge.DurationDays,
		Reward:         challenge.Reward,
		RewardImageURL: challenge.RewardImageUrl,
		CreatedAt:      fromPgTimestamptz(challenge.CreatedAt),
	}
}
