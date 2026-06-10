package http

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	fasthttpws "github.com/fasthttp/websocket"
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"one-more-mile/server/internal/geo"
	"one-more-mile/server/internal/http/middleware"
	db "one-more-mile/server/internal/sqlc"
	"one-more-mile/server/internal/util"
)

const (
	metersPerMile         = 1609.344
	milesPerMeter         = 1 / metersPerMile
	metersPerSecondPerMPH = 0.44704
	streamReadLimitBytes  = 4096
	streamReadTimeout     = 75 * time.Second
	streamWriteTimeout    = 10 * time.Second
	streamPingInterval    = 30 * time.Second
	dbProgressFlushEvery  = 10

	streamTypeProgress           = "progress"
	streamTypeChallengeCompleted = "challenge_completed"
	streamTypeError              = "error"
)

type sessionStreamMeta struct {
	SessionID      uuid.UUID `json:"session_id"`
	UserID         uuid.UUID `json:"user_id"`
	ChallengeID    uuid.UUID `json:"challenge_id"`
	ChallengeTitle string    `json:"challenge_title"`
	TargetMiles    float64   `json:"target_miles"`
	HMACSecret     string    `json:"hmac_secret"`
	StartLat       float64   `json:"start_lat"`
	StartLng       float64   `json:"start_lng"`
	StepsStart     int32     `json:"steps_start"`
	MilesStart     float64   `json:"miles_start"`
	DistanceMiles  float64   `json:"distance_miles"`
	ExpiresAt      time.Time `json:"expires_at"`
}

type sessionStreamState struct {
	LastLat       float64   `json:"last_lat"`
	LastLng       float64   `json:"last_lng"`
	LastTimestamp time.Time `json:"last_timestamp"`
	LastSeq       int64     `json:"last_seq"`
	StepsStart    int32     `json:"steps_start"`
	StepsLast     int32     `json:"steps_last"`
	DistanceMiles float64   `json:"distance_miles"`
	Completed     bool      `json:"completed"`
	Initialized   bool      `json:"initialized"`
}

type streamPointMessage struct {
	Lat       float64   `json:"lat" validate:"required"`
	Lng       float64   `json:"lng" validate:"required"`
	Timestamp time.Time `json:"timestamp" validate:"required"`
	Seq       int64     `json:"seq" validate:"required"`
	Steps     int32     `json:"steps"`
	Signature string    `json:"signature" validate:"required"`
}

type streamProgressMessage struct {
	Type          string    `json:"type"`
	SessionID     uuid.UUID `json:"session_id"`
	ChallengeID   uuid.UUID `json:"challenge_id"`
	DistanceMiles float64   `json:"distance_miles"`
	TargetMiles   float64   `json:"target_miles"`
	Steps         int32     `json:"steps"`
	Seq           int64     `json:"seq"`
	Completed     bool      `json:"completed"`
	CouponCode    string    `json:"coupon_code,omitempty"`
	CouponStatus  string    `json:"coupon_status,omitempty"`
	Message       string    `json:"message,omitempty"`
}

type streamErrorMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type completedChallengeResult struct {
	CouponCode   string
	CouponStatus string
}

func (h *Handler) AuthWebSocket(c *fiber.Ctx) error {
	if !websocket.IsWebSocketUpgrade(c) {
		return fiber.ErrUpgradeRequired
	}

	tokenValue := middleware.BearerToken(c.Get("Authorization"))
	if tokenValue == "" {
		tokenValue = strings.TrimSpace(c.Query("token"))
	}
	if tokenValue == "" {
		tokenValue = tokenFromSubprotocol(c.Get("Sec-WebSocket-Protocol"))
	}
	if tokenValue == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing websocket token"})
	}

	claims, err := middleware.ParseToken(h.cfg, tokenValue)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid token"})
	}

	c.Locals(middleware.ClaimsKey, claims)
	return c.Next()
}

func (h *Handler) StreamSession(conn *websocket.Conn) {
	claims, ok := conn.Locals(middleware.ClaimsKey).(middleware.Claims)
	if !ok {
		_ = writeStreamError(conn, "unauthorized")
		return
	}

	sessionID, err := uuid.Parse(conn.Params("id"))
	if err != nil {
		_ = writeStreamError(conn, "invalid session id")
		return
	}

	ctx := context.Background()
	meta, err := h.getSessionStreamMeta(ctx, sessionID, claims.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			_ = writeStreamError(conn, "active session not found")
			return
		}
		_ = writeStreamError(conn, "failed to load session")
		return
	}

	state, err := h.getSessionStreamState(ctx, meta)
	if err != nil {
		_ = writeStreamError(conn, "failed to load session stream state")
		return
	}

	conn.SetReadLimit(streamReadLimitBytes)
	_ = conn.SetReadDeadline(time.Now().Add(streamReadTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(streamReadTimeout))
	})

	stopPing := make(chan struct{})
	go func() {
		ticker := time.NewTicker(streamPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = conn.SetWriteDeadline(time.Now().Add(streamWriteTimeout))
				if err := conn.WriteMessage(fasthttpws.PingMessage, nil); err != nil {
					return
				}
			case <-stopPing:
				return
			}
		}
	}()
	defer close(stopPing)

	acceptedSinceFlush := 0
	for {
		var point streamPointMessage
		if err := conn.ReadJSON(&point); err != nil {
			break
		}

		result, accepted, err := h.acceptStreamPoint(ctx, meta, &state, point)
		if err != nil {
			_ = writeStreamError(conn, err.Error())
			continue
		}
		if accepted {
			acceptedSinceFlush++
		}

		message := streamProgressMessage{
			Type:          streamTypeProgress,
			SessionID:     meta.SessionID,
			ChallengeID:   meta.ChallengeID,
			DistanceMiles: roundMiles(state.DistanceMiles),
			TargetMiles:   meta.TargetMiles,
			Steps:         state.StepsLast,
			Seq:           state.LastSeq,
			Completed:     state.Completed,
		}

		if result != nil {
			message.Type = streamTypeChallengeCompleted
			message.Completed = true
			message.CouponCode = result.CouponCode
			message.CouponStatus = result.CouponStatus
			message.Message = fmt.Sprintf("challenge #%s complete!", meta.ChallengeID.String())
		}

		if err := writeStreamJSON(conn, message); err != nil {
			break
		}

		if acceptedSinceFlush >= dbProgressFlushEvery {
			_ = h.db.UpdateSessionProgress(ctx, db.UpdateSessionProgressParams{
				ID:            meta.SessionID,
				DistanceMiles: state.DistanceMiles,
			})
			acceptedSinceFlush = 0
		}
	}

	if acceptedSinceFlush > 0 {
		_ = h.db.UpdateSessionProgress(ctx, db.UpdateSessionProgressParams{
			ID:            meta.SessionID,
			DistanceMiles: state.DistanceMiles,
		})
	}
}

func (h *Handler) acceptStreamPoint(ctx context.Context, meta sessionStreamMeta, state *sessionStreamState, point streamPointMessage) (*completedChallengeResult, bool, error) {
	if err := validateStreamPointBasics(point); err != nil {
		return nil, false, err
	}
	if !validPointSignature(meta.SessionID, meta.HMACSecret, point) {
		return nil, false, errors.New("invalid point signature")
	}

	now := time.Now()
	if point.Timestamp.Before(now.Add(-h.cfg.MaxGPSPointAge)) {
		return nil, false, errors.New("gps point is too old")
	}
	if point.Timestamp.After(now.Add(h.cfg.MaxGPSPointFutureSkew)) {
		return nil, false, errors.New("gps point timestamp is in the future")
	}

	if state.Initialized {
		if point.Seq <= state.LastSeq {
			return nil, false, errors.New("gps point sequence is not increasing")
		}
		if !point.Timestamp.After(state.LastTimestamp) {
			return nil, false, errors.New("gps point timestamp is not increasing")
		}
		if point.Steps < state.StepsLast {
			return nil, false, errors.New("steps must be non-decreasing")
		}
	} else if point.Steps < state.StepsStart {
		return nil, false, errors.New("steps must be greater than or equal to steps_start")
	}

	deltaMeters := 0.0
	speedMps := 0.0
	speedViolation := false
	if state.Initialized {
		seconds := point.Timestamp.Sub(state.LastTimestamp).Seconds()
		if seconds <= 0 {
			return nil, false, errors.New("invalid gps point interval")
		}

		stepDelta := int32(0)
		if point.Steps > state.StepsLast {
			stepDelta = point.Steps - state.StepsLast
		}
		if h.cfg.MaxStepsPerMinute > 0 {
			stepsPerMinute := float64(stepDelta) / seconds * 60
			if stepsPerMinute > h.cfg.MaxStepsPerMinute {
				return nil, false, errors.New("unrealistic step rate")
			}
		}

		deltaMeters = geo.HaversineMeters(state.LastLat, state.LastLng, point.Lat, point.Lng)
		speedMps = deltaMeters / seconds
		maxSpeedMps := h.cfg.MaxGPSSpeedMPH * metersPerSecondPerMPH
		if maxSpeedMps > 0 && speedMps > maxSpeedMps {
			speedViolation = true
			deltaMeters = 0
		}

		if h.cfg.MaxStrideLengthMeters > 0 && deltaMeters > 0 {
			if stepDelta > 0 {
				if (deltaMeters / float64(stepDelta)) > h.cfg.MaxStrideLengthMeters {
					speedViolation = true
					deltaMeters = 0
				}
			} else if deltaMeters > 10 {
				// Moved more than 10 meters without a single step
				speedViolation = true
				deltaMeters = 0
			}
		}
	}

	if _, err := h.db.CreateSessionCheckpoint(ctx, db.CreateSessionCheckpointParams{
		SessionID:      meta.SessionID,
		Lat:            point.Lat,
		Lng:            point.Lng,
		RecordedAt:     toPgTimestamptz(point.Timestamp),
		Steps:          point.Steps,
		DistanceMeters: deltaMeters,
		SpeedMps:       speedMps,
		SpeedViolation: speedViolation,
	}); err != nil {
		return nil, false, errors.New("failed to save gps checkpoint")
	}

	state.LastLat = point.Lat
	state.LastLng = point.Lng
	state.LastTimestamp = point.Timestamp
	state.LastSeq = point.Seq
	state.StepsLast = point.Steps
	state.DistanceMiles += deltaMeters * milesPerMeter
	state.Initialized = true

	if err := h.saveSessionStreamState(ctx, meta.SessionID, *state); err != nil {
		return nil, false, errors.New("failed to save session stream state")
	}

	if !state.Completed && state.DistanceMiles >= meta.TargetMiles {
		result, err := h.completeChallengeFromStream(ctx, meta, *state)
		if err != nil {
			return nil, true, err
		}
		state.Completed = true
		_ = h.saveSessionStreamState(ctx, meta.SessionID, *state)
		return result, true, nil
	}

	return nil, true, nil
}

func (h *Handler) getSessionStreamMeta(ctx context.Context, sessionID, userID uuid.UUID) (sessionStreamMeta, error) {
	key := sessionStreamMetaKey(sessionID)
	if raw, err := h.redis.Get(ctx, key).Result(); err == nil {
		var meta sessionStreamMeta
		if json.Unmarshal([]byte(raw), &meta) == nil && meta.SessionID == sessionID && meta.UserID == userID {
			return meta, nil
		}
	} else if !errors.Is(err, redis.Nil) {
		return sessionStreamMeta{}, err
	}

	row, err := h.db.GetActiveSessionStreamMeta(ctx, db.GetActiveSessionStreamMetaParams{
		ID:     sessionID,
		UserID: userID,
	})
	if err != nil {
		return sessionStreamMeta{}, err
	}

	meta := sessionStreamMeta{
		SessionID:      row.ID,
		UserID:         row.UserID,
		ChallengeID:    row.ChallengeID,
		ChallengeTitle: row.ChallengeTitle,
		TargetMiles:    row.TargetMiles,
		HMACSecret:     row.HmacSecret,
		StartLat:       row.StartLat,
		StartLng:       row.StartLng,
		StepsStart:     row.StepsStart,
		MilesStart:     row.MilesStart,
		DistanceMiles:  row.DistanceMiles,
		ExpiresAt:      fromPgTimestamptz(row.ExpiresAt),
	}
	if meta.TargetMiles <= 0 {
		return sessionStreamMeta{}, errors.New("invalid challenge target")
	}

	_ = h.saveSessionStreamMeta(ctx, meta)
	return meta, nil
}

func (h *Handler) getSessionStreamState(ctx context.Context, meta sessionStreamMeta) (sessionStreamState, error) {
	key := sessionStreamStateKey(meta.SessionID)
	if raw, err := h.redis.Get(ctx, key).Result(); err == nil {
		var state sessionStreamState
		if json.Unmarshal([]byte(raw), &state) == nil {
			return state, nil
		}
	} else if !errors.Is(err, redis.Nil) {
		return sessionStreamState{}, err
	}

	state := sessionStreamState{
		LastLat:       meta.StartLat,
		LastLng:       meta.StartLng,
		StepsStart:    meta.StepsStart,
		StepsLast:     meta.StepsStart,
		DistanceMiles: meta.DistanceMiles,
		Initialized:   false,
	}
	return state, h.saveSessionStreamState(ctx, meta.SessionID, state)
}

func (h *Handler) saveSessionStreamMeta(ctx context.Context, meta sessionStreamMeta) error {
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return h.redis.Set(ctx, sessionStreamMetaKey(meta.SessionID), raw, h.cfg.SessionStreamTTL).Err()
}

func (h *Handler) saveSessionStreamState(ctx context.Context, sessionID uuid.UUID, state sessionStreamState) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return h.redis.Set(ctx, sessionStreamStateKey(sessionID), raw, h.cfg.SessionStreamTTL).Err()
}

func (h *Handler) loadSessionStreamState(ctx context.Context, sessionID uuid.UUID) (sessionStreamState, bool, error) {
	raw, err := h.redis.Get(ctx, sessionStreamStateKey(sessionID)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return sessionStreamState{}, false, nil
		}
		return sessionStreamState{}, false, err
	}

	var state sessionStreamState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return sessionStreamState{}, false, err
	}

	return state, true, nil
}

func (h *Handler) cleanupSessionStream(ctx context.Context, sessionID uuid.UUID) {
	_ = h.redis.Del(ctx,
		sessionStreamMetaKey(sessionID),
		sessionStreamStateKey(sessionID),
		sessionStreamCompletionLockKey(sessionID),
	).Err()
}

func (h *Handler) completeChallengeFromStream(ctx context.Context, meta sessionStreamMeta, state sessionStreamState) (*completedChallengeResult, error) {
	lockKey := sessionStreamCompletionLockKey(meta.SessionID)
	locked, err := h.redis.SetNX(ctx, lockKey, "1", h.cfg.SessionStreamTTL).Result()
	if err != nil {
		return nil, errors.New("failed to lock challenge completion")
	}

	if !locked {
		coupon, err := h.db.GetCouponByUserChallenge(ctx, db.GetCouponByUserChallengeParams{
			UserID:      meta.UserID,
			ChallengeID: meta.ChallengeID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, nil
			}
			return nil, errors.New("failed to load issued coupon")
		}
		return &completedChallengeResult{CouponCode: coupon.Code, CouponStatus: coupon.Status}, nil
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return nil, errors.New("failed to start completion transaction")
	}
	defer tx.Rollback(ctx)

	queries := h.db.WithTx(tx)
	if err := queries.UpdateSessionProgress(ctx, db.UpdateSessionProgressParams{
		ID:            meta.SessionID,
		DistanceMiles: state.DistanceMiles,
	}); err != nil {
		return nil, errors.New("failed to update session progress")
	}

	if err := queries.MarkChallengeRegistrationCompletedIfActive(ctx, db.MarkChallengeRegistrationCompletedIfActiveParams{
		UserID:      meta.UserID,
		ChallengeID: meta.ChallengeID,
	}); err != nil {
		return nil, errors.New("failed to finalize challenge")
	}

	code, err := util.RandomCouponCode(10)
	if err != nil {
		return nil, errors.New("failed to issue coupon")
	}

	coupon, err := queries.CreateCouponIfNotExists(ctx, db.CreateCouponIfNotExistsParams{
		ID:          uuid.New(),
		UserID:      meta.UserID,
		ChallengeID: meta.ChallengeID,
		SessionID:   meta.SessionID,
		Code:        code,
		Status:      "active",
	})
	if err != nil {
		return nil, errors.New("failed to issue coupon")
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, errors.New("failed to commit challenge completion")
	}

	return &completedChallengeResult{CouponCode: coupon.Code, CouponStatus: coupon.Status}, nil
}

func validateStreamPointBasics(point streamPointMessage) error {
	if math.IsNaN(point.Lat) || math.IsInf(point.Lat, 0) || point.Lat < -90 || point.Lat > 90 {
		return errors.New("invalid latitude")
	}
	if math.IsNaN(point.Lng) || math.IsInf(point.Lng, 0) || point.Lng < -180 || point.Lng > 180 {
		return errors.New("invalid longitude")
	}
	if point.Timestamp.IsZero() {
		return errors.New("missing timestamp")
	}
	if point.Seq <= 0 {
		return errors.New("seq must be positive")
	}
	if point.Steps < 0 {
		return errors.New("steps must be non-negative")
	}
	if point.Signature == "" {
		return errors.New("missing signature")
	}
	return nil
}

func validPointSignature(sessionID uuid.UUID, secret string, point streamPointMessage) bool {
	expected := util.HMACSHA256Hex(secret, []byte(pointSignaturePayload(sessionID, point)))
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(point.Signature)), []byte(expected)) == 1
}

func pointSignaturePayload(sessionID uuid.UUID, point streamPointMessage) string {
	return strings.Join([]string{
		sessionID.String(),
		point.Timestamp.UTC().Format(time.RFC3339Nano),
		strconv.FormatFloat(point.Lat, 'f', 7, 64),
		strconv.FormatFloat(point.Lng, 'f', 7, 64),
		strconv.FormatInt(point.Seq, 10),
		strconv.FormatInt(int64(point.Steps), 10),
	}, "|")
}

func writeStreamError(conn *websocket.Conn, message string) error {
	return writeStreamJSON(conn, streamErrorMessage{Type: streamTypeError, Message: message})
}

func writeStreamJSON(conn *websocket.Conn, value interface{}) error {
	_ = conn.SetWriteDeadline(time.Now().Add(streamWriteTimeout))
	return conn.WriteJSON(value)
}

func tokenFromSubprotocol(header string) string {
	for _, part := range strings.Split(header, ",") {
		value := strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(value), "bearer.") {
			return strings.TrimSpace(value[len("bearer."):])
		}
	}
	return ""
}

func sessionStreamMetaKey(sessionID uuid.UUID) string {
	return "session:" + sessionID.String() + ":meta"
}

func sessionStreamStateKey(sessionID uuid.UUID) string {
	return "session:" + sessionID.String() + ":gps"
}

func sessionStreamCompletionLockKey(sessionID uuid.UUID) string {
	return "session:" + sessionID.String() + ":completed"
}

func roundMiles(value float64) float64 {
	return math.Round(value*10000) / 10000
}
