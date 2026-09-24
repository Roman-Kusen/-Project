package main

import "time"

type Service struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	MonthlyPrice float64 `json:"monthly_price"`
}

type Subscription struct {
	ID            int64     `json:"id"`
	UserID        int64     `json:"user_id"`
	ServiceID     int64     `json:"service_id"`
	ServiceName   string    `json:"service_name"`
	Price         float64   `json:"price"`
	Status        string    `json:"status"`
	StartedAt     time.Time `json:"started_at"`
	NextPaymentAt time.Time `json:"next_payment_at"`
}

type Payment struct {
	ID             int64     `json:"id"`
	SubscriptionID int64     `json:"subscription_id"`
	Amount         float64   `json:"amount"`
	PaidAt         time.Time `json:"paid_at"`
}
