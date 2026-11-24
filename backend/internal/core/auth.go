package core

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/config"
	"github.com/reusing-code/kontor/backend/internal/email"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/module"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

const (
	minPasswordLength = 8
	// bcrypt only takes 72 bytes and refuses longer input.
	maxPasswordBytes = 72
)

// checkNewPassword returns the client error for an unusable new password.
func checkNewPassword(password string) string {
	switch {
	case len(password) < minPasswordLength:
		return "password must be at least 8 characters"
	case len(password) > maxPasswordBytes:
		return "password must be at most 72 bytes"
	}
	return ""
}

// SeedFunc initializes default data for a new or returning user.
type SeedFunc func(ctx context.Context, userID string) error

// EnabledOnlySeed wraps a module seed so it only runs for users who have the
// module enabled.
func EnabledOnlySeed(store *Store, moduleID string, seed SeedFunc) SeedFunc {
	return func(ctx context.Context, userID string) error {
		enabled, err := store.ModuleEnabled(ctx, userID, moduleID)
		if err != nil {
			return err
		}
		if !enabled {
			return nil
		}
		return seed(ctx, userID)
	}
}

type Handler struct {
	store          *Store
	logger         *slog.Logger
	jwtSecret      []byte
	emailClient    *email.Client
	seeds          []SeedFunc
	registry       *module.Registry
	paperlessLinks PaperlessLinkStore
	capabilities   Capabilities
	registration   config.Registration
	background     *sync.WaitGroup
	// registerMu makes the first-user check and the create one step.
	registerMu sync.Mutex
}

func NewHandler(store *Store, logger *slog.Logger, jwtSecret []byte, emailClient *email.Client, seeds []SeedFunc, registry *module.Registry) *Handler {
	return &Handler{
		store:       store,
		logger:      logger,
		jwtSecret:   jwtSecret,
		emailClient: emailClient,
		seeds:       seeds,
		registry:    registry,
		background:  &sync.WaitGroup{},
	}
}

// SetBackground makes the welcome mail count as background work that
// shutdown waits for.
func (h *Handler) SetBackground(wg *sync.WaitGroup) { h.background = wg }

// SetRegistration sets who may register; the zero value is
// config.RegistrationFirstUser.
func (h *Handler) SetRegistration(r config.Registration) { h.registration = r }

func (h *Handler) registrationOpen(ctx context.Context) (bool, error) {
	switch h.registration {
	case config.RegistrationOpen:
		return true, nil
	case config.RegistrationClosed:
		return false, nil
	}
	hasUsers, err := h.store.HasUsers(ctx)
	return !hasUsers, err
}

type authConfigResponse struct {
	RegistrationOpen bool `json:"registrationOpen"`
}

// AuthConfig tells the login page whether it may offer registration.
func (h *Handler) AuthConfig(w http.ResponseWriter, r *http.Request) {
	open, err := h.registrationOpen(r.Context())
	if err != nil {
		h.logger.Error("checking registration", "error", err)
		httputil.Error(h.logger, w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httputil.WriteJSON(h.logger, w, http.StatusOK, authConfigResponse{RegistrationOpen: open})
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
	h.registerMu.Lock()
	defer h.registerMu.Unlock()
	open, err := h.registrationOpen(r.Context())
	if err != nil {
		h.logger.Error("checking registration", "error", err)
		httputil.Error(h.logger, w, http.StatusInternalServerError, "internal error")
		return
	}
	if !open {
		httputil.WriteJSON(h.logger, w, http.StatusForbidden, map[string]string{"error": "registration is closed", "code": "registration_closed"})
		return
	}

	var req authRequest
	if err := httputil.ReadJSON(r, &req); err != nil {
		httputil.BodyError(h.logger, w, err, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		httputil.Error(h.logger, w, http.StatusBadRequest, "email and password are required")
		return
	}
	if msg := checkNewPassword(req.Password); msg != "" {
		httputil.Error(h.logger, w, http.StatusBadRequest, msg)
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
		h.background.Go(func() { h.sendWelcomeEmail(user.Email) })
	}

	token, err := h.issueToken(user)
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
		httputil.BodyError(h.logger, w, err, "invalid request body")
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

	token, err := h.issueToken(user)
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

func (h *Handler) issueToken(user User) (string, error) {
	claims := middleware.JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		TokenVersion: user.TokenVersion,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(h.jwtSecret)
}

func (h *Handler) sendWelcomeEmail(to string) {
	subject := "Welcome to Kontor!"
	body := "Hello,\n\nWelcome to Kontor! You have successfully registered your account.\n\nBest regards,\nYour Kontor Team"

	if err := h.emailClient.Send([]string{to}, subject, body); err != nil {
		h.logger.Error("sending welcome email", "error", err)
	}
}
