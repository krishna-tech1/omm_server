package http

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"

	db "one-more-mile/server/internal/sqlc"
)

func (h *Handler) RunMaintenanceWorker(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// Run once immediately
	h.runMaintenanceTask(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.runMaintenanceTask(ctx)
		}
	}
}

func (h *Handler) runMaintenanceTask(ctx context.Context) {
	// 1. Expire overdue challenge registrations
	if err := h.db.ExpireOverdueRegistrations(ctx); err != nil {
		log.Printf("Maintenance worker: failed to expire overdue registrations: %v", err)
	}

	// 2. Clean up stale sessions
	timeoutMinutes := h.cfg.StaleSessionTimeoutMinutes
	if timeoutMinutes <= 0 {
		timeoutMinutes = 30 // default fallback
	}

	staleSessions, err := h.db.FindStaleSessions(ctx, int32(timeoutMinutes))
	if err != nil {
		log.Printf("Maintenance worker: failed to find stale sessions: %v", err)
		return
	}

	for _, session := range staleSessions {
		h.autoCloseSession(ctx, session)
	}
}

func (h *Handler) autoCloseSession(ctx context.Context, session db.Session) {
	log.Printf("Maintenance worker: auto-closing stale session %s for user %s", session.ID, session.UserID)

	// Persist the distance from the stream state if available, otherwise use DB's distance_miles
	distanceMiles := session.DistanceMiles
	streamState, hasStreamState, err := h.loadSessionStreamState(ctx, session.ID)
	if err == nil && hasStreamState && streamState.Initialized {
		distanceMiles = streamState.DistanceMiles
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		log.Printf("Maintenance worker: failed to start tx for session %s: %v", session.ID, err)
		return
	}
	defer tx.Rollback(ctx)

	queries := h.db.WithTx(tx)

	if err := queries.UpdateSessionStatusAndDistance(ctx, db.UpdateSessionStatusAndDistanceParams{
		ID:            session.ID,
		Status:        "completed",
		DistanceMiles: distanceMiles,
	}); err != nil {
		log.Printf("Maintenance worker: failed to update session status %s: %v", session.ID, err)
		return
	}

	if err := queries.AddRegistrationDistance(ctx, db.AddRegistrationDistanceParams{
		ChallengeID:     session.ChallengeID,
		UserID:          session.UserID,
		DistanceCovered: distanceMiles,
	}); err != nil {
		log.Printf("Maintenance worker: failed to add registration distance %s: %v", session.ID, err)
		return
	}

	// Check if challenge is completed
	challenge, err := queries.GetChallengeByID(ctx, session.ChallengeID)
	if err != nil {
		log.Printf("Maintenance worker: failed to load challenge %s: %v", session.ChallengeID, err)
		return
	}

	registration, err := queries.GetChallengeRegistrationForUser(ctx, db.GetChallengeRegistrationForUserParams{
		ChallengeID: session.ChallengeID,
		UserID:      session.UserID,
	})
	if err != nil {
		log.Printf("Maintenance worker: failed to load registration %s: %v", session.ChallengeID, err)
		return
	}

	if registration.DistanceCovered >= challenge.TargetMiles && registration.Status != "completed" {
		if err := queries.MarkChallengeRegistrationCompletedIfActive(ctx, db.MarkChallengeRegistrationCompletedIfActiveParams{
			UserID:      session.UserID,
			ChallengeID: session.ChallengeID,
		}); err != nil {
			log.Printf("Maintenance worker: failed to complete registration %s: %v", session.ChallengeID, err)
			return
		}

		// Issue coupon
		_, err = queries.CreateCoupon(ctx, db.CreateCouponParams{
			ID:          uuid.New(),
			Code:        uuid.New().String()[:8],
			ChallengeID: session.ChallengeID,
			SessionID:   session.ID,
			UserID:      session.UserID,
			Status:      "active",
		})
		if err != nil {
			log.Printf("Maintenance worker: failed to create coupon %s: %v", session.ChallengeID, err)
			// Don't return, we still want to commit the progress
		}
	}

	if err := tx.Commit(ctx); err != nil {
		log.Printf("Maintenance worker: failed to commit tx %s: %v", session.ID, err)
		return
	}

	// Clean up Redis keys
	h.redis.Del(ctx, sessionStreamMetaKey(session.ID))
	h.redis.Del(ctx, sessionStreamStateKey(session.ID))
	h.redis.Set(ctx, sessionStreamCompletionLockKey(session.ID), "1", 24*time.Hour)
}
