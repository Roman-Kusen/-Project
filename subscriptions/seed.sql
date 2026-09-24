INSERT INTO users (name, email) VALUES
    ('Аня',   'anya@example.com'),
    ('Борис', 'boris@example.com');

INSERT INTO services (name, monthly_price) VALUES
    ('Spotify',        199.00),
    ('Яндекс Плюс',    299.00),
    ('Хостинг VPS',    450.00),
    ('Онлайн-курсы',  1990.00);

-- Аня: Spotify (уже давно) + хостинг
INSERT INTO subscriptions (user_id, service_id, price, status, started_at, next_payment_at) VALUES
    (1, 1, 199.00, 'active',  now() - interval '3 months', now() + interval '5 days'),
    (1, 3, 450.00, 'active',  now() - interval '1 month',  now() + interval '20 days'),
    (1, 4, 1990.00,'paused',  now() - interval '2 months', now() + interval '40 days');

-- Борис: Яндекс Плюс (скоро спишется) + Spotify (отменён)
INSERT INTO subscriptions (user_id, service_id, price, status, started_at, next_payment_at) VALUES
    (2, 2, 299.00, 'active',    now() - interval '10 days', now() + interval '3 days'),
    (2, 1, 199.00, 'cancelled', now() - interval '6 months', now() - interval '5 months');

INSERT INTO payments (subscription_id, amount, paid_at) VALUES
    (1, 199.00, now() - interval '3 months'),
    (1, 199.00, now() - interval '2 months'),
    (1, 199.00, now() - interval '1 month'),
    (4, 299.00, now() - interval '10 days');