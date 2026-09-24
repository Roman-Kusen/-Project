package main

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

// --- users ---

func (s *Store) UserExists(ctx context.Context, id int64) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, id).Scan(&exists)
	return exists, err
}

// --- services ---

func (s *Store) ListServices(ctx context.Context) ([]Service, error) {
	rows, err := s.db.Query(ctx, `SELECT id, name, monthly_price FROM services ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	services := []Service{}
	for rows.Next() {
		var svc Service
		if err := rows.Scan(&svc.ID, &svc.Name, &svc.MonthlyPrice); err != nil {
			return nil, err
		}
		services = append(services, svc)
	}
	return services, rows.Err()
}

func (s *Store) GetService(ctx context.Context, id int64) (Service, error) {
	var svc Service
	err := s.db.QueryRow(ctx,
		`SELECT id, name, monthly_price FROM services WHERE id = $1`, id).
		Scan(&svc.ID, &svc.Name, &svc.MonthlyPrice)
	if errors.Is(err, pgx.ErrNoRows) {
		return Service{}, ErrNotFound
	}
	if err != nil {
		return Service{}, err
	}
	return svc, nil
}

func (s *Store) FindServiceByName(ctx context.Context, name string) (Service, error) {
	var svc Service
	err := s.db.QueryRow(ctx,
		`SELECT id, name, monthly_price FROM services WHERE name = $1`, name).
		Scan(&svc.ID, &svc.Name, &svc.MonthlyPrice)
	if errors.Is(err, pgx.ErrNoRows) {
		return Service{}, ErrNotFound
	}
	if err != nil {
		return Service{}, err
	}
	return svc, nil
}

func (s *Store) CreateService(ctx context.Context, name string, price float64) (Service, error) {
	var svc Service
	err := s.db.QueryRow(ctx,
		`INSERT INTO services (name, monthly_price)
		 VALUES ($1, $2)
		 RETURNING id, name, monthly_price`,
		name, price).
		Scan(&svc.ID, &svc.Name, &svc.MonthlyPrice)
	if err != nil {
		return Service{}, err
	}
	return svc, nil
}

// --- subscriptions ---

func (s *Store) ListSubscriptions(ctx context.Context, userID int64) ([]Subscription, error) {
	rows, err := s.db.Query(ctx, `
		SELECT s.id, s.user_id, s.service_id, svc.name, s.price, s.status,
		       s.started_at, s.next_payment_at
		FROM subscriptions s
		JOIN services svc ON svc.id = s.service_id
		WHERE s.user_id = $1
		ORDER BY s.next_payment_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	subs := []Subscription{}
	for rows.Next() {
		var sub Subscription
		if err := rows.Scan(&sub.ID, &sub.UserID, &sub.ServiceID, &sub.ServiceName,
			&sub.Price, &sub.Status, &sub.StartedAt, &sub.NextPaymentAt); err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (s *Store) GetSubscription(ctx context.Context, id int64) (Subscription, error) {
	var sub Subscription
	err := s.db.QueryRow(ctx, `
		SELECT s.id, s.user_id, s.service_id, svc.name, s.price, s.status,
		       s.started_at, s.next_payment_at
		FROM subscriptions s
		JOIN services svc ON svc.id = s.service_id
		WHERE s.id = $1`, id).
		Scan(&sub.ID, &sub.UserID, &sub.ServiceID, &sub.ServiceName,
			&sub.Price, &sub.Status, &sub.StartedAt, &sub.NextPaymentAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Subscription{}, ErrNotFound
	}
	if err != nil {
		return Subscription{}, err
	}
	return sub, nil
}

// CreateSubscription — принимает явные даты начала и следующего списания.
func (s *Store) CreateSubscription(
	ctx context.Context,
	userID, serviceID int64,
	startedAt, nextPaymentAt time.Time,
) (Subscription, error) {
	var sub Subscription
	err := s.db.QueryRow(ctx, `
		INSERT INTO subscriptions (user_id, service_id, price, started_at, next_payment_at)
		SELECT $1, id, monthly_price, $3, $4
		FROM services WHERE id = $2
		RETURNING id, user_id, service_id, price, status, started_at, next_payment_at`,
		userID, serviceID, startedAt, nextPaymentAt).
		Scan(&sub.ID, &sub.UserID, &sub.ServiceID, &sub.Price, &sub.Status,
			&sub.StartedAt, &sub.NextPaymentAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Subscription{}, ErrNotFound
	}
	if err != nil {
		return Subscription{}, err
	}
	if svc, err := s.GetService(ctx, sub.ServiceID); err == nil {
		sub.ServiceName = svc.Name
	}
	return sub, nil
}

// CreateSubscriptionByName — «найти или создать сервис» + подписка с датами.
func (s *Store) CreateSubscriptionByName(
	ctx context.Context,
	userID int64,
	serviceName string,
	price float64,
	startedAt, nextPaymentAt time.Time,
) (Subscription, error) {
	svc, err := s.FindServiceByName(ctx, serviceName)
	if errors.Is(err, ErrNotFound) {
		svc, err = s.CreateService(ctx, serviceName, price)
	}
	if err != nil {
		return Subscription{}, err
	}
	return s.CreateSubscription(ctx, userID, svc.ID, startedAt, nextPaymentAt)
}

func (s *Store) UpdateSubscriptionStatus(ctx context.Context, id int64, status string) (Subscription, error) {
	var sub Subscription
	err := s.db.QueryRow(ctx, `
		UPDATE subscriptions SET status = $1 WHERE id = $2
		RETURNING id, user_id, service_id, price, status, started_at, next_payment_at`,
		status, id).
		Scan(&sub.ID, &sub.UserID, &sub.ServiceID, &sub.Price, &sub.Status,
			&sub.StartedAt, &sub.NextPaymentAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Subscription{}, ErrNotFound
	}
	if err != nil {
		return Subscription{}, err
	}
	if svc, err := s.GetService(ctx, sub.ServiceID); err == nil {
		sub.ServiceName = svc.Name
	}
	return sub, nil
}

func (s *Store) DeleteSubscription(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM subscriptions WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// --- upcoming ---

func (s *Store) UpcomingPayments(ctx context.Context, userID int64) ([]Subscription, error) {
	rows, err := s.db.Query(ctx, `
		SELECT s.id, s.user_id, s.service_id, svc.name, s.price, s.status,
		       s.started_at, s.next_payment_at
		FROM subscriptions s
		JOIN services svc ON svc.id = s.service_id
		WHERE s.user_id = $1
		  AND s.status = 'active'
		  AND s.next_payment_at < now() + interval '7 days'
		ORDER BY s.next_payment_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	subs := []Subscription{}
	for rows.Next() {
		var sub Subscription
		if err := rows.Scan(&sub.ID, &sub.UserID, &sub.ServiceID, &sub.ServiceName,
			&sub.Price, &sub.Status, &sub.StartedAt, &sub.NextPaymentAt); err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

// --- monthly total ---

func (s *Store) MonthlyTotal(ctx context.Context, userID int64) (float64, error) {
	var total float64
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(price), 0)
		FROM subscriptions
		WHERE user_id = $1 AND status = 'active'`, userID).Scan(&total)
	return total, err
}

// --- payments ---

func (s *Store) ListPayments(ctx context.Context, subscriptionID int64) ([]Payment, error) {
	if _, err := s.GetSubscription(ctx, subscriptionID); err != nil {
		return nil, err
	}

	rows, err := s.db.Query(ctx, `
		SELECT id, subscription_id, amount, paid_at
		FROM payments WHERE subscription_id = $1
		ORDER BY paid_at DESC`, subscriptionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	payments := []Payment{}
	for rows.Next() {
		var p Payment
		if err := rows.Scan(&p.ID, &p.SubscriptionID, &p.Amount, &p.PaidAt); err != nil {
			return nil, err
		}
		payments = append(payments, p)
	}
	return payments, rows.Err()
}

func (s *Store) CreatePayment(ctx context.Context, subscriptionID int64, amount float64) (Payment, error) {
	var p Payment
	err := s.db.QueryRow(ctx, `
		INSERT INTO payments (subscription_id, amount)
		VALUES ($1, $2)
		RETURNING id, subscription_id, amount, paid_at`,
		subscriptionID, amount).
		Scan(&p.ID, &p.SubscriptionID, &p.Amount, &p.PaidAt)
	if err != nil {
		return Payment{}, err
	}
	return p, nil
}
