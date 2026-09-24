package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Handler struct {
	store *Store
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func pathID(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(r.PathValue(name), 10, 64)
}

// --- services ---

func (h *Handler) listServices(w http.ResponseWriter, r *http.Request) {
	services, err := h.store.ListServices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, services)
}

// --- subscriptions ---

func (h *Handler) listSubscriptions(w http.ResponseWriter, r *http.Request) {
	userID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a number")
		return
	}

	exists, err := h.store.UserExists(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	subs, err := h.store.ListSubscriptions(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, subs)
}

type createSubscriptionRequest struct {
	ServiceID     int64   `json:"service_id"`
	ServiceName   string  `json:"service_name"`
	Price         float64 `json:"price"`
	StartedAt     string  `json:"started_at"`      // RFC3339, опционально
	NextPaymentAt string  `json:"next_payment_at"` // RFC3339, опционально
}

func (h *Handler) createSubscription(w http.ResponseWriter, r *http.Request) {
	userID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a number")
		return
	}

	var req createSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	req.ServiceName = strings.TrimSpace(req.ServiceName)

	if req.ServiceID == 0 && req.ServiceName == "" {
		writeError(w, http.StatusUnprocessableEntity, "service_id or service_name is required")
		return
	}
	if req.ServiceID != 0 && req.ServiceName != "" {
		writeError(w, http.StatusUnprocessableEntity, "specify either service_id or service_name, not both")
		return
	}
	if req.ServiceName != "" {
		if len([]rune(req.ServiceName)) > 100 {
			writeError(w, http.StatusUnprocessableEntity, "service_name is too long")
			return
		}
		if req.Price < 0 {
			writeError(w, http.StatusUnprocessableEntity, "price must be >= 0")
			return
		}
	}

	// --- парсинг и валидация дат ---

	now := time.Now()
	startedAt := now
	nextPaymentAt := now.AddDate(0, 1, 0)

	if req.StartedAt != "" {
		t, err := time.Parse(time.RFC3339, req.StartedAt)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "started_at must be RFC3339 (e.g. 2024-03-15T10:00:00Z)")
			return
		}
		startedAt = t
	}
	if req.NextPaymentAt != "" {
		t, err := time.Parse(time.RFC3339, req.NextPaymentAt)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "next_payment_at must be RFC3339")
			return
		}
		nextPaymentAt = t
	}

	// started_at не может быть в будущем (с запасом 24 часа на часовые пояса)
	if startedAt.After(now.Add(24 * time.Hour)) {
		writeError(w, http.StatusUnprocessableEntity, "started_at cannot be in the future")
		return
	}
	// next_payment_at должен быть позже started_at
	if !nextPaymentAt.After(startedAt) {
		writeError(w, http.StatusUnprocessableEntity, "next_payment_at must be after started_at")
		return
	}
	// разумные границы
	if startedAt.Before(now.AddDate(-10, 0, 0)) {
		writeError(w, http.StatusUnprocessableEntity, "started_at is too far in the past")
		return
	}
	if nextPaymentAt.After(now.AddDate(3, 0, 0)) {
		writeError(w, http.StatusUnprocessableEntity, "next_payment_at is too far in the future")
		return
	}

	// --- проверка пользователя ---

	exists, err := h.store.UserExists(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	// --- создание ---

	var sub Subscription

	if req.ServiceID != 0 {
		sub, err = h.store.CreateSubscription(r.Context(), userID, req.ServiceID, startedAt, nextPaymentAt)
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "service not found")
			return
		}
	} else {
		sub, err = h.store.CreateSubscriptionByName(r.Context(), userID, req.ServiceName, req.Price, startedAt, nextPaymentAt)
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusCreated, sub)
}

type updateSubscriptionRequest struct {
	Status string `json:"status"`
}

var allowedStatuses = map[string]bool{
	"active": true, "paused": true, "cancelled": true, "expired": true,
}

func (h *Handler) patchSubscription(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a number")
		return
	}

	var req updateSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.Status = strings.TrimSpace(req.Status)
	if !allowedStatuses[req.Status] {
		writeError(w, http.StatusUnprocessableEntity, "invalid status")
		return
	}

	sub, err := h.store.UpdateSubscriptionStatus(r.Context(), id, req.Status)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, sub)
}

func (h *Handler) deleteSubscription(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a number")
		return
	}

	err = h.store.DeleteSubscription(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- upcoming ---

func (h *Handler) upcomingPayments(w http.ResponseWriter, r *http.Request) {
	userID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a number")
		return
	}

	exists, err := h.store.UserExists(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	subs, err := h.store.UpcomingPayments(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, subs)
}

// --- monthly total ---

func (h *Handler) monthlyTotal(w http.ResponseWriter, r *http.Request) {
	userID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a number")
		return
	}

	exists, err := h.store.UserExists(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	total, err := h.store.MonthlyTotal(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]float64{"total": total})
}

// --- payments ---

func (h *Handler) listPayments(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a number")
		return
	}

	payments, err := h.store.ListPayments(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, payments)
}

type createPaymentRequest struct {
	Amount float64 `json:"amount"`
}

func (h *Handler) createPayment(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a number")
		return
	}

	var req createPaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Amount <= 0 {
		writeError(w, http.StatusUnprocessableEntity, "amount must be positive")
		return
	}

	if _, err := h.store.GetSubscription(r.Context(), id); errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	p, err := h.store.CreatePayment(r.Context(), id, req.Amount)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusCreated, p)
}
