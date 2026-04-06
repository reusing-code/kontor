package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/tobi/contracts/backend/internal/model"
	"github.com/tobi/contracts/backend/internal/store"
	"golang.org/x/crypto/bcrypt"
)

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	Token string     `json:"token"`
	User  model.User `json:"user"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := h.readJSON(r, &req); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		h.errorResponse(w, http.StatusBadRequest, "email and password are required")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		h.logger.Error("hashing password", "error", err)
		h.errorResponse(w, http.StatusInternalServerError, "internal error")
		return
	}

	user := model.User{
		ID:           uuid.New(),
		Email:        req.Email,
		PasswordHash: string(hash),
		CreatedAt:    time.Now().UTC(),
	}

	if err := h.store.CreateUser(r.Context(), user); err != nil {
		if err == store.ErrConflict {
			h.errorResponse(w, http.StatusConflict, "email already registered")
			return
		}
		h.logger.Error("creating user", "error", err)
		h.errorResponse(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.seedDefaultCategoriesIfEmpty(r.Context(), user.ID.String(), "contracts", defaultContractCategories)
	h.seedDefaultCategoriesIfEmpty(r.Context(), user.ID.String(), "purchases", defaultPurchaseCategories)
	h.seedDefaultLedgerCategoriesIfEmpty(r.Context(), user.ID.String())

	if h.emailClient != nil {
		go h.sendWelcomeEmail(user.Email)
	}

	token, err := h.issueToken(user.ID.String())
	if err != nil {
		h.logger.Error("issuing token", "error", err)
		h.errorResponse(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.writeJSON(w, http.StatusCreated, authResponse{Token: token, User: user})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := h.readJSON(r, &req); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		h.errorResponse(w, http.StatusBadRequest, "email and password are required")
		return
	}

	user, err := h.store.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		if err == store.ErrNotFound {
			h.errorResponse(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		h.logger.Error("looking up user", "error", err)
		h.errorResponse(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		h.errorResponse(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	h.seedDefaultCategoriesIfEmpty(r.Context(), user.ID.String(), "contracts", defaultContractCategories)
	h.seedDefaultCategoriesIfEmpty(r.Context(), user.ID.String(), "purchases", defaultPurchaseCategories)
	h.seedDefaultLedgerCategoriesIfEmpty(r.Context(), user.ID.String())

	token, err := h.issueToken(user.ID.String())
	if err != nil {
		h.logger.Error("issuing token", "error", err)
		h.errorResponse(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.writeJSON(w, http.StatusOK, authResponse{Token: token, User: user})
}

var defaultContractCategories = []struct {
	Name    string
	NameKey string
}{
	{"Insurance", "categoryNames.insurance"},
	{"Banking / Portfolios", "categoryNames.banking"},
	{"Telecommunications", "categoryNames.telecommunications"},
}

var defaultPurchaseCategories = []struct {
	Name    string
	NameKey string
}{
	{"PC Hardware", "categoryNames.pcHardware"},
	{"Entertainment", "categoryNames.entertainment"},
	{"Kitchen", "categoryNames.kitchen"},
	{"Tools", "categoryNames.tools"},
	{"Household", "categoryNames.household"},
}

func (h *Handler) seedDefaultCategoriesIfEmpty(ctx context.Context, userID string, module string, defaults []struct {
	Name    string
	NameKey string
}) {
	existing, err := h.store.ListCategories(ctx, userID, module)
	if err != nil {
		h.logger.Error("listing categories for seed check", "module", module, "error", err)
		return
	}
	if len(existing) > 0 {
		return
	}

	now := time.Now().UTC()
	for _, dc := range defaults {
		cat := model.Category{
			ID:        uuid.New(),
			Name:      dc.Name,
			NameKey:   dc.NameKey,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := h.store.CreateCategory(ctx, userID, module, cat); err != nil {
			h.logger.Error("seeding default category", "module", module, "name", dc.Name, "error", err)
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
	subject := "Welcome to Contracts!"
	body := "Hello,\n\nWelcome to Contracts! You have successfully registered your account.\n\nBest regards,\nYour Contracts Team"

	if err := h.emailClient.Send([]string{to}, subject, body); err != nil {
		h.logger.Error("sending welcome email", "to", to, "error", err)
	}
}
