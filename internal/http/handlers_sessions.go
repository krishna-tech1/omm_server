package http

import (
	"context"
	"sort"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	db "one-more-mile/server/internal/sqlc"
	"one-more-mile/server/internal/util"
)

type startSessionRequest struct {
	ChallengeIDs []uuid.UUID `json:"challenge_ids"`
	StartTime    time.Time   `json:"start_time" validate:"required"`
	StartLat     float64     `json:"start_lat" validate:"required"`
	StartLng     float64     `json:"start_lng" validate:"required"`
	StepsStart   int32       `json:"steps_start"`
	MilesStart   float64     `json:"miles_start"`
}

type startSessionResult struct {
	SessionID   uuid.UUID `json:"session_id"`
	ChallengeID uuid.UUID `json:"challenge_id"`
	HMACSecret  string    `json:"hmac_secret"`
	Status      string    `json:"status"`
	StartTime   time.Time `json:"start_time"`
}

type startSessionResponse struct {
	Sessions []startSessionResult `json:"sessions"`
}

func (h *Handler) StartSession(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}

	var req startSessionRequest
	if err := h.parseBody(c, &req); err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid request")
	}

	ctx, cancel := h.requestContext()
	defer cancel()

	requestedIDs := uniqueUUIDs(req.ChallengeIDs)
	if len(requestedIDs) > 0 {
		now := time.Now()
		for _, challengeID := range requestedIDs {
			challenge, err := h.db.GetChallengeByID(ctx, challengeID)
			if err != nil {
				if err == pgx.ErrNoRows {
					return h.respondError(c, fiber.StatusNotFound, "challenge not found")
				}
				return h.respondError(c, fiber.StatusInternalServerError, "failed to load challenge")
			}
			if !fromPgTimestamptz(challenge.ExpiresAt).After(now) {
				return h.respondError(c, fiber.StatusBadRequest, "challenge expired")
			}
		}
	}

	activeRegistered, err := h.db.ListActiveChallengeIDsForUser(ctx, db.ListActiveChallengeIDsForUserParams{
		UserID:    claims.UserID,
		ExpiresAt: toPgTimestamptz(time.Now()),
	})
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load challenges")
	}

	startSet := make(map[uuid.UUID]struct{}, len(requestedIDs)+len(activeRegistered))
	for _, id := range requestedIDs {
		startSet[id] = struct{}{}
	}
	for _, id := range activeRegistered {
		startSet[id] = struct{}{}
	}

	if len(startSet) == 0 {
		return h.respondError(c, fiber.StatusBadRequest, "no active challenges to start")
	}

	ctx, cancel = h.requestContext()
	defer cancel()

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to start transaction")
	}
	defer tx.Rollback(ctx)

	queries := h.db.WithTx(tx)

	for _, challengeID := range requestedIDs {
		if _, err := queries.RegisterChallenge(ctx, db.RegisterChallengeParams{
			ID:          uuid.New(),
			ChallengeID: challengeID,
			UserID:      claims.UserID,
		}); err != nil {
			return h.respondError(c, fiber.StatusInternalServerError, "failed to register challenge")
		}
	}

	startIDs := make([]uuid.UUID, 0, len(startSet))
	for id := range startSet {
		startIDs = append(startIDs, id)
	}
	sort.Slice(startIDs, func(i, j int) bool {
		return startIDs[i].String() < startIDs[j].String()
	})

	response := startSessionResponse{
		Sessions: make([]startSessionResult, 0, len(startIDs)),
	}

	for _, challengeID := range startIDs {
		secret, err := util.RandomHex(32)
		if err != nil {
			return h.respondError(c, fiber.StatusInternalServerError, "failed to create session")
		}

		session, err := queries.CreateSession(ctx, db.CreateSessionParams{
			ID:          uuid.New(),
			UserID:      claims.UserID,
			ChallengeID: challengeID,
			StartTime:   toPgTimestamptz(req.StartTime),
			StartLat:    req.StartLat,
			StartLng:    req.StartLng,
			StepsStart:  req.StepsStart,
			MilesStart:  req.MilesStart,
			Status:      "active",
			HmacSecret:  secret,
		})
		if err != nil {
			return h.respondError(c, fiber.StatusInternalServerError, "failed to start session")
		}

		response.Sessions = append(response.Sessions, startSessionResult{
			SessionID:   session.ID,
			ChallengeID: session.ChallengeID,
			HMACSecret:  session.HmacSecret,
			Status:      session.Status,
			StartTime:   fromPgTimestamptz(session.StartTime),
		})
	}

	if err := tx.Commit(ctx); err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to commit session")
	}

	return c.JSON(response)
}

type stopSessionRequest struct {
	SessionIDs []uuid.UUID `json:"session_ids" validate:"required"`
	EndTime    time.Time   `json:"end_time" validate:"required"`
	EndLat     float64     `json:"end_lat" validate:"required"`
	EndLng     float64     `json:"end_lng" validate:"required"`
	StepsEnd   int32       `json:"steps_end"`
	TotalMiles float64     `json:"total_miles" validate:"required"`
}

type stopSessionResult struct {
	SessionID     uuid.UUID `json:"session_id"`
	ChallengeID   uuid.UUID `json:"challenge_id"`
	Status        string    `json:"status"`
	DistanceMiles float64   `json:"distance_miles"`
	CouponCode    string    `json:"coupon_code,omitempty"`
	CouponStatus  string    `json:"coupon_status,omitempty"`
}

type stopSessionResponse struct {
	Sessions []stopSessionResult `json:"sessions"`
}

func (h *Handler) StopSession(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}

	var req stopSessionRequest
	if err := h.parseBody(c, &req); err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid request")
	}

	if len(req.SessionIDs) == 0 {
		return h.respondError(c, fiber.StatusBadRequest, "missing session ids")
	}
	if req.TotalMiles < 0 {
		return h.respondError(c, fiber.StatusBadRequest, "invalid total miles")
	}

	sessionIDs := uniqueUUIDs(req.SessionIDs)

	ctx, cancel := h.requestContext()
	defer cancel()

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to start transaction")
	}
	defer tx.Rollback(ctx)

	queries := h.db.WithTx(tx)

	sessions := make([]db.Session, 0, len(sessionIDs))
	challengeTargets := make(map[uuid.UUID]float64)
	for _, sessionID := range sessionIDs {
		session, err := queries.GetSessionForUser(ctx, db.GetSessionForUserParams{
			ID:     sessionID,
			UserID: claims.UserID,
		})
		if err != nil {
			if err == pgx.ErrNoRows {
				return h.respondError(c, fiber.StatusNotFound, "session not found")
			}
			return h.respondError(c, fiber.StatusInternalServerError, "failed to load session")
		}
		if session.Status != "active" && session.Status != "completed" {
			return h.respondError(c, fiber.StatusBadRequest, "session is not active")
		}
		sessions = append(sessions, session)

		if _, ok := challengeTargets[session.ChallengeID]; !ok {
			challenge, err := queries.GetChallengeByID(ctx, session.ChallengeID)
			if err != nil {
				return h.respondError(c, fiber.StatusInternalServerError, "failed to load challenge")
			}
			challengeTargets[session.ChallengeID] = challenge.TargetMiles
		}
	}

	response := stopSessionResponse{
		Sessions: make([]stopSessionResult, 0, len(sessions)),
	}

	streamCleanupIDs := make([]uuid.UUID, 0, len(sessions))
	for _, session := range sessions {
		distanceMiles := req.TotalMiles
		endLat := req.EndLat
		endLng := req.EndLng
		stepsEnd := req.StepsEnd
		milesEnd := req.TotalMiles

		streamState, hasStreamState, err := h.loadSessionStreamState(ctx, session.ID)
		if err != nil {
			return h.respondError(c, fiber.StatusInternalServerError, "failed to load session stream")
		}
		if hasStreamState && streamState.Initialized {
			distanceMiles = streamState.DistanceMiles
			endLat = streamState.LastLat
			endLng = streamState.LastLng
			stepsEnd = streamState.StepsLast
			milesEnd = session.MilesStart + streamState.DistanceMiles
			streamCleanupIDs = append(streamCleanupIDs, session.ID)
		}

		if distanceMiles < 0 {
			return h.respondError(c, fiber.StatusBadRequest, "invalid total miles")
		}

		if session.Status == "completed" {
			result := stopSessionResult{
				SessionID:     session.ID,
				ChallengeID:   session.ChallengeID,
				Status:        "completed",
				DistanceMiles: session.DistanceMiles,
			}
			coupon, err := queries.GetCouponByUserChallenge(ctx, db.GetCouponByUserChallengeParams{
				UserID:      claims.UserID,
				ChallengeID: session.ChallengeID,
			})
			if err == nil {
				result.CouponCode = coupon.Code
				result.CouponStatus = coupon.Status
			} else if err != pgx.ErrNoRows {
				return h.respondError(c, fiber.StatusInternalServerError, "failed to load coupon")
			}
			response.Sessions = append(response.Sessions, result)
			continue
		}

		if err := queries.UpdateSessionEnd(ctx, db.UpdateSessionEndParams{
			ID:       session.ID,
			EndTime:  toPgTimestamptz(req.EndTime),
			EndLat:   endLat,
			EndLng:   endLng,
			StepsEnd: stepsEnd,
			MilesEnd: milesEnd,
		}); err != nil {
			return h.respondError(c, fiber.StatusInternalServerError, "failed to update session")
		}

		if err := queries.UpdateSessionStatusAndDistance(ctx, db.UpdateSessionStatusAndDistanceParams{
			ID:            session.ID,
			Status:        "completed",
			DistanceMiles: distanceMiles,
		}); err != nil {
			return h.respondError(c, fiber.StatusInternalServerError, "failed to finalize session")
		}

		result := stopSessionResult{
			SessionID:     session.ID,
			ChallengeID:   session.ChallengeID,
			Status:        "completed",
			DistanceMiles: distanceMiles,
		}

		if targetMiles, ok := challengeTargets[session.ChallengeID]; ok && distanceMiles >= targetMiles {
			code, err := util.RandomCouponCode(10)
			if err != nil {
				return h.respondError(c, fiber.StatusInternalServerError, "failed to issue coupon")
			}

			coupon, err := queries.CreateCouponIfNotExists(ctx, db.CreateCouponIfNotExistsParams{
				ID:          uuid.New(),
				UserID:      claims.UserID,
				ChallengeID: session.ChallengeID,
				SessionID:   session.ID,
				Code:        code,
				Status:      "active",
			})
			if err != nil {
				return h.respondError(c, fiber.StatusInternalServerError, "failed to issue coupon")
			}

			if err := queries.MarkChallengeRegistrationCompletedIfActive(ctx, db.MarkChallengeRegistrationCompletedIfActiveParams{
				UserID:      claims.UserID,
				ChallengeID: session.ChallengeID,
			}); err != nil {
				return h.respondError(c, fiber.StatusInternalServerError, "failed to finalize challenge")
			}

			result.CouponCode = coupon.Code
			result.CouponStatus = coupon.Status
		}

		response.Sessions = append(response.Sessions, result)
	}

	if err := tx.Commit(ctx); err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to commit session")
	}

	for _, sessionID := range streamCleanupIDs {
		h.cleanupSessionStream(context.Background(), sessionID)
	}

	return c.JSON(response)
}

func uniqueUUIDs(values []uuid.UUID) []uuid.UUID {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[uuid.UUID]struct{}, len(values))
	unique := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}

	return unique
}
