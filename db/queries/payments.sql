-- name: CreatePayment :one
INSERT INTO payments (user_id, stripe_payment_intent_id, amount_cents, currency, status)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetMRR :one
SELECT COALESCE(SUM(amount_cents), 0)::bigint AS mrr_cents
FROM payments
WHERE status = 'succeeded'
  AND created_at >= NOW() - INTERVAL '30 days';
