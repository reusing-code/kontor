package core

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/email"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

const minPasswordLength = 8

// SeedFunc initializes default data for a new or returning user.
type SeedFunc func(ctx context.Context, userID string) error

type Handler struct {
	store       *Store
	logger      *slog.Logger
	jwtSecret   []byte
	emailClient *email.Client
	seeds       []SeedFunc
}

func NewHandler(store *Store, logger *slog.Logger, jwtSecret []byte, emailClient *email.Client, seeds []SeedFunc) *Handler {
	return &Handler{
		store:       store,
		logger:      logger,
		jwtSecret:   jwtSecret,
		emailClient: emailClient,
		seeds:       seeds,
	}
}

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := httputil.ReadJSON(r, &req); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		httputil.Error(h.logger, w, http.StatusBadRequest, "email and password are required")
		return
	}
	if len(req.Password) < minPasswordLength {
		httputil.Error(h.logger, w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		h.logger.Error("hashing password", "error", err)
		httputil.Error(h.logger, w, http.StatusInternalServerError, "internal error")
		return
	}

	user := User{
		ID:           uuid.New(),
		Email:        req.Email,
		PasswordHash: string(hash),
		CreatedAt:    time.Now().UTC(),
	}

	if err := h.store.CreateUser(r.Context(), user); err != nil {
		if errors.Is(err, storage.ErrConflict) {
			httputil.Error(h.logger, w, http.StatusConflict, "email already registered")
			return
		}
		h.logger.Error("creating user", "error", err)
		httputil.Error(h.logger, w, http.StatusInternalServerError, "internal error")
		return
	}

	h.runSeeds(r.Context(), user.ID.String())

	if h.emailClient != nil {
		go h.sendWelcomeEmail(user.Email)
	}

	token, err := h.issueToken(user.ID.String())
	if err != nil {
		h.logger.Error("issuing token", "error", err)
		httputil.Error(h.logger, w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(h.logger, w, http.StatusCreated, authResponse{Token: token, User: user})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := httputil.ReadJSON(r, &req); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		httputil.Error(h.logger, w, http.StatusBadRequest, "email and password are required")
		return
	}

	user, err := h.store.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			httputil.Error(h.logger, w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		h.logger.Error("looking up user", "error", err)
		httputil.Error(h.logger, w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		httputil.Error(h.logger, w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	h.runSeeds(r.Context(), user.ID.String())

	token, err := h.issueToken(user.ID.String())
	if err != nil {
		h.logger.Error("issuing token", "error", err)
		httputil.Error(h.logger, w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(h.logger, w, http.StatusOK, authResponse{Token: token, User: user})
}

func (h *Handler) runSeeds(ctx context.Context, userID string) {
	for _, seed := range h.seeds {
		if err := seed(ctx, userID); err != nil {
			h.logger.Error("seeding defaults", "user", userID, "error", err)
		}
	}
}

func (h *Handler) issueToken(userID string) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(h.jwtSecret)
}

func (h *Handler) sendWelcomeEmail(to string) {
	subject := "Welcome to Kontor!"
	body := "Hello,\n\nWelcome to Kontor! You have successfully registered your account.\n\nBest regards,\nYour Kontor Team"

	if err := h.emailClient.Send([]string{to}, subject, body); err != nil {
		h.logger.Error("sending welcome email", "to", to, "error", err)
	}
}
