CREATE TABLE IF NOT EXISTS users (
    id    BIGSERIAL PRIMARY KEY,
    name  TEXT NOT NULL,
    email TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS services (
    id            BIGSERIAL      PRIMARY KEY,
    name          TEXT           NOT NULL UNIQUE,
    monthly_price NUMERIC(10, 2) NOT NULL CHECK (monthly_price >= 0)
);

CREATE TABLE IF NOT EXISTS subscriptions (
    id              BIGSERIAL      PRIMARY KEY,
    user_id         BIGINT         NOT NULL REFERENCES users(id)    ON DELETE CASCADE,
    service_id      BIGINT         NOT NULL REFERENCES services(id) ON DELETE RESTRICT,
    price           NUMERIC(10, 2) NOT NULL CHECK (price >= 0),
    status          TEXT           NOT NULL DEFAULT 'active'
                      CHECK (status IN ('active', 'paused', 'cancelled', 'expired')),
    started_at      TIMESTAMPTZ    NOT NULL DEFAULT now(),
    next_payment_at TIMESTAMPTZ    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_subscriptions_user_id         ON subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_service_id      ON subscriptions(service_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_next_payment_at ON subscriptions(next_payment_at);

CREATE TABLE IF NOT EXISTS payments (
    id              BIGSERIAL      PRIMARY KEY,
    subscription_id BIGINT         NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    amount          NUMERIC(10, 2) NOT NULL CHECK (amount >= 0),
    paid_at         TIMESTAMPTZ    NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_payments_subscription_id ON payments(subscription_id);
CREATE INDEX IF NOT EXISTS idx_payments_paid_at         ON payments(paid_at);