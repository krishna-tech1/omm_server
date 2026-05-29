package http

import (
	"crypto/subtle"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"one-more-mile/server/internal/geo"
	db "one-more-mile/server/internal/sqlc"
	"one-more-mile/server/internal/util"
)

const (
	maxSpeedMps        = 6.26
	stepRatioThreshold = 500.0
	metersPerMile      = 1609.344
)

type startSessionRequest struct {
	ChallengeID uuid.UUID `json:"challenge_id" validate:"required"`
	StartTime   time.Time `json:"start_time" validate:"required"`
	StartLat    float64   `json:"start_lat" validate:"required"`
	StartLng    float64   `json:"start_lng" validate:"required"`
	StepsStart  int32     `json:"steps_start"`
	MilesStart  float64   `json:"miles_start"`
}

type startSessionResponse struct {
	SessionID  uuid.UUID `json:"session_id"`
	HMACSecret string    `json:"hmac_secret"`
	Status     string    `json:"status"`
	StartTime  time.Time `json:"start_time"`
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

	secret, err := util.RandomHex(32)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to create session")
	}

	ctx, cancel := h.requestContext()
	defer cancel()

	session, err := h.db.CreateSession(ctx, db.CreateSessionParams{
		ID:          uuid.New(),
		UserID:      claims.UserID,
		ChallengeID: req.ChallengeID,
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

	return c.JSON(startSessionResponse{
		SessionID:  session.ID,
		HMACSecret: session.HmacSecret,
		Status:     session.Status,
		StartTime:  fromPgTimestamptz(session.StartTime),
	})
}

type checkpoint struct {
	Lat        float64   `json:"lat" validate:"required"`
	Lng        float64   `json:"lng" validate:"required"`
	RecordedAt time.Time `json:"recorded_at" validate:"required"`
	Steps      int32     `json:"steps"`
}

type uploadCheckpointsRequest struct {
	Points []checkpoint `json:"points" validate:"required,min=1,dive"`
}

func (h *Handler) UploadCheckpoints(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}

	sessionID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid session id")
	}

	ctx, cancel := h.requestContext()
	defer cancel()

	session, err := h.db.GetSessionForUser(ctx, db.GetSessionForUserParams{
		ID:     sessionID,
		UserID: claims.UserID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return h.respondError(c, fiber.StatusNotFound, "session not found")
		}
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load session")
	}

	if session.Status != "active" {
		return h.respondError(c, fiber.StatusBadRequest, "session is not active")
	}

	signature := c.Get("X-Signature")
	if signature == "" {
		return h.respondError(c, fiber.StatusUnauthorized, "missing signature")
	}

	expected := util.HMACSHA256Hex(session.HmacSecret, c.Body())
	if subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) != 1 {
		return h.respondError(c, fiber.StatusUnauthorized, "invalid signature")
	}

	var req uploadCheckpointsRequest
	if err := h.parseBody(c, &req); err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid request")
	}

	var prevLat float64
	var prevLng float64
	var prevTime time.Time

	last, err := h.db.GetLastCheckpoint(ctx, sessionID)
	if err != nil && err != pgx.ErrNoRows {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load checkpoints")
	}
	if err == nil {
		prevLat = last.Lat
		prevLng = last.Lng
		prevTime = fromPgTimestamptz(last.RecordedAt)
	}

	for _, point := range req.Points {
		distance := 0.0
		speed := 0.0
		violation := false

		if !prevTime.IsZero() {
			distance = geo.HaversineMeters(prevLat, prevLng, point.Lat, point.Lng)
			dt := point.RecordedAt.Sub(prevTime).Seconds()
			if dt <= 0 {
				distance = 0
				violation = true
			} else {
				speed = distance / dt
				if speed > maxSpeedMps {
					distance = 0
					violation = true
				}
			}
		}

		err = h.db.CreateCheckpoint(ctx, db.CreateCheckpointParams{
			SessionID:      sessionID,
			Lat:            point.Lat,
			Lng:            point.Lng,
			RecordedAt:     toPgTimestamptz(point.RecordedAt),
			Steps:          point.Steps,
			DistanceMeters: distance,
			SpeedMps:       speed,
			SpeedViolation: violation,
		})
		if err != nil {
			return h.respondError(c, fiber.StatusInternalServerError, "failed to store checkpoints")
		}

		prevLat = point.Lat
		prevLng = point.Lng
		prevTime = point.RecordedAt
	}

	return c.JSON(fiber.Map{"inserted": len(req.Points)})
}

type stopSessionRequest struct {
	SessionID uuid.UUID `json:"session_id" validate:"required"`
	EndTime   time.Time `json:"end_time" validate:"required"`
	EndLat    float64   `json:"end_lat" validate:"required"`
	EndLng    float64   `json:"end_lng" validate:"required"`
	StepsEnd  int32     `json:"steps_end"`
	MilesEnd  float64   `json:"miles_end"`
}

type stopSessionResponse struct {
	SessionID     uuid.UUID `json:"session_id"`
	Status        string    `json:"status"`
	DistanceMiles float64   `json:"distance_miles"`
	StepsDelta    int32     `json:"steps_delta"`
	StepRatio     float64   `json:"step_ratio"`
	VoucherCode   string    `json:"voucher_code,omitempty"`
	VoucherStatus string    `json:"voucher_status,omitempty"`
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

	ctx, cancel := h.requestContext()
	defer cancel()

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to start transaction")
	}
	defer tx.Rollback(ctx)

	queries := h.db.WithTx(tx)

	session, err := queries.GetSessionForUser(ctx, db.GetSessionForUserParams{
		ID:     req.SessionID,
		UserID: claims.UserID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return h.respondError(c, fiber.StatusNotFound, "session not found")
		}
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load session")
	}

	if session.Status != "active" {
		return h.respondError(c, fiber.StatusBadRequest, "session is not active")
	}

	if err := queries.UpdateSessionEnd(ctx, db.UpdateSessionEndParams{
		ID:       req.SessionID,
		EndTime:  toPgTimestamptz(req.EndTime),
		EndLat:   req.EndLat,
		EndLng:   req.EndLng,
		StepsEnd: req.StepsEnd,
		MilesEnd: req.MilesEnd,
	}); err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to update session")
	}

	totalMeters, err := queries.SumCheckpointDistanceMeters(ctx, req.SessionID)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to calculate distance")
	}

	distanceMiles := totalMeters / metersPerMile
	stepsDelta := req.StepsEnd - session.StepsStart
	stepRatio := 0.0
	suspicious := false

	if distanceMiles > 0 {
		stepRatio = float64(stepsDelta) / distanceMiles
	}

	if stepsDelta < 0 {
		suspicious = true
	} else if stepsDelta == 0 && distanceMiles > 0 {
		suspicious = true
	} else if stepsDelta > 0 && stepRatio < stepRatioThreshold {
		suspicious = true
	}

	status := "completed"
	voucherStatus := "active"
	if suspicious {
		status = "suspicious"
		voucherStatus = "pending"
	}

	if err := queries.UpdateSessionStatusAndDistance(ctx, db.UpdateSessionStatusAndDistanceParams{
		ID:            req.SessionID,
		Status:        status,
		DistanceMiles: distanceMiles,
	}); err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to finalize session")
	}

	challenge, err := queries.GetChallengeByID(ctx, session.ChallengeID)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load challenge")
	}

	response := stopSessionResponse{
		SessionID:     req.SessionID,
		Status:        status,
		DistanceMiles: distanceMiles,
		StepsDelta:    stepsDelta,
		StepRatio:     stepRatio,
	}

	if distanceMiles >= challenge.TargetMiles {
		code, err := util.RandomVoucherCode(10)
		if err != nil {
			return h.respondError(c, fiber.StatusInternalServerError, "failed to issue voucher")
		}

		voucher, err := queries.CreateVoucher(ctx, db.CreateVoucherParams{
			ID:          uuid.New(),
			UserID:      claims.UserID,
			ChallengeID: challenge.ID,
			SessionID:   session.ID,
			Code:        code,
			Status:      voucherStatus,
		})
		if err != nil {
			return h.respondError(c, fiber.StatusInternalServerError, "failed to issue voucher")
		}

		response.VoucherCode = voucher.Code
		response.VoucherStatus = voucher.Status
	}

	if err := tx.Commit(ctx); err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to commit session")
	}

	return c.JSON(response)
}
