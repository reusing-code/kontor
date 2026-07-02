package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/model"
	"github.com/reusing-code/kontor/backend/internal/modules/auto"
	"github.com/reusing-code/kontor/backend/internal/modules/contracts"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

var (
	ErrNotFound = storage.ErrNotFound
	ErrConflict = storage.ErrConflict
)

type BackupConfig = storage.BackupConfig

type BadgerStore struct {
	engine    *storage.Engine
	db        *badger.DB
	logger    *slog.Logger
	links     *link.Registry
	contracts *contracts.Store
	auto      *auto.Store
}

// New assembles the transitional store facade over the shared engine and the
// already-split module stores. It registers itself as the ledger transaction
// side of the link registry.
func New(engine *storage.Engine, links *link.Registry, contractsStore *contracts.Store, autoStore *auto.Store, logger *slog.Logger) *BadgerStore {
	s := &BadgerStore{
		engine:    engine,
		db:        engine.DB(),
		logger:    logger,
		links:     links,
		contracts: contractsStore,
		auto:      autoStore,
	}
	links.SetTransactionSide(s)
	links.RegisterTarget(link.RefPurchase, PurchaseLinkTarget{})
	return s
}

func (s *BadgerStore) Engine() *storage.Engine {
	return s.engine
}

func (s *BadgerStore) Close() error {
	return s.engine.Close()
}

func (s *BadgerStore) Healthy() error {
	return s.engine.Healthy()
}

func (s *BadgerStore) Backup(w io.Writer) error {
	return s.engine.Backup(w)
}

func (s *BadgerStore) StartBackups(ctx context.Context, cfg BackupConfig) {
	s.engine.StartBackups(ctx, cfg)
}

// User keys

func usrKey(id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("usr/%s", id))
}

func usrEmailKey(email string) []byte {
	return []byte(fmt.Sprintf("usr_email/%s", email))
}

// storableUser includes PasswordHash for persistence (model.User has json:"-" on it).
type storableUser struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"passwordHash"`
	CreatedAt    time.Time `json:"createdAt"`
}

func toStorableUser(u model.User) storableUser {
	return storableUser{
		ID:           u.ID,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		CreatedAt:    u.CreatedAt,
	}
}

func (su storableUser) toModel() model.User {
	return model.User{
		ID:           su.ID,
		Email:        su.Email,
		PasswordHash: su.PasswordHash,
		CreatedAt:    su.CreatedAt,
	}
}

// Users

func (s *BadgerStore) CreateUser(_ context.Context, u model.User) error {
	data, err := json.Marshal(toStorableUser(u))
	if err != nil {
		return err
	}
	return s.db.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(usrEmailKey(u.Email)); err == nil {
			return ErrConflict
		}
		if err := txn.Set(usrKey(u.ID), data); err != nil {
			return err
		}
		return txn.Set(usrEmailKey(u.Email), []byte(u.ID.String()))
	})
}

func (s *BadgerStore) GetUserByID(_ context.Context, id string) (model.User, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return model.User{}, ErrNotFound
	}
	var user model.User
	err = s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(usrKey(uid))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			var su storableUser
			if err := json.Unmarshal(val, &su); err != nil {
				return err
			}
			user = su.toModel()
			return nil
		})
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return user, ErrNotFound
	}
	return user, err
}

func (s *BadgerStore) UpdateUser(_ context.Context, u model.User) error {
	data, err := json.Marshal(toStorableUser(u))
	if err != nil {
		return err
	}
	return s.db.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(usrKey(u.ID)); err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrNotFound
			}
			return err
		}
		return txn.Set(usrKey(u.ID), data)
	})
}

func (s *BadgerStore) ListUsers(_ context.Context) ([]model.User, error) {
	var users []model.User
	prefix := []byte("usr/")

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var su storableUser
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &su)
			}); err != nil {
				return err
			}
			users = append(users, su.toModel())
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if users == nil {
		users = []model.User{}
	}
	return users, nil
}

func settingsKey(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/settings", userID))
}

func (s *BadgerStore) GetSettings(_ context.Context, userID string) (model.UserSettings, error) {
	var settings model.UserSettings
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(settingsKey(userID))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &settings)
		})
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return model.DefaultUserSettings(), nil
	}
	return settings, err
}

func (s *BadgerStore) UpdateSettings(_ context.Context, userID string, st model.UserSettings) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(settingsKey(userID), data)
	})
}

func (s *BadgerStore) GetUserByEmail(_ context.Context, email string) (model.User, error) {
	var user model.User
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(usrEmailKey(email))
		if err != nil {
			return err
		}
		var idStr string
		if err := item.Value(func(val []byte) error {
			idStr = string(val)
			return nil
		}); err != nil {
			return err
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return err
		}
		uItem, err := txn.Get(usrKey(id))
		if err != nil {
			return err
		}
		return uItem.Value(func(val []byte) error {
			var su storableUser
			if err := json.Unmarshal(val, &su); err != nil {
				return err
			}
			user = su.toModel()
			return nil
		})
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return user, ErrNotFound
	}
	return user, err
}

// Module-scoped category key helpers
// Key format: u/{userID}/mod/{module}/cat/{categoryID}

func modCatKey(userID, module string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/%s/cat/%s", userID, module, id))
}

func modCatPrefix(userID, module string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/%s/cat/", userID, module))
}

// Purchase key helpers
// Key format: u/{userID}/pur/{purchaseID}
// Index: u/{userID}/idx/cat_pur/{categoryID}/{purchaseID}

func purKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/pur/%s", userID, id))
}

func purPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/pur/", userID))
}

func idxCatPurKey(userID string, categoryID, purchaseID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/cat_pur/%s/%s", userID, categoryID, purchaseID))
}

func idxCatPurPrefix(userID string, categoryID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/cat_pur/%s/", userID, categoryID))
}

// Categories (module-scoped)

func (s *BadgerStore) ListCategories(_ context.Context, userID string, module string) ([]model.Category, error) {
	var categories []model.Category
	prefix := modCatPrefix(userID, module)

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var cat model.Category
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &cat)
			}); err != nil {
				return err
			}
			categories = append(categories, cat)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if categories == nil {
		categories = []model.Category{}
	}
	return categories, nil
}

func (s *BadgerStore) GetCategory(_ context.Context, userID string, module string, id uuid.UUID) (model.Category, error) {
	var cat model.Category
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(modCatKey(userID, module, id))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &cat)
		})
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return cat, ErrNotFound
	}
	return cat, err
}

func (s *BadgerStore) CreateCategory(_ context.Context, userID string, module string, c model.Category) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(modCatKey(userID, module, c.ID), data)
	})
}

func (s *BadgerStore) UpdateCategory(_ context.Context, userID string, module string, c model.Category) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return s.db.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(modCatKey(userID, module, c.ID)); err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrNotFound
			}
			return err
		}
		return txn.Set(modCatKey(userID, module, c.ID), data)
	})
}

func (s *BadgerStore) DeleteCategory(_ context.Context, userID string, module string, id uuid.UUID) error {
	return s.db.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(modCatKey(userID, module, id)); err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrNotFound
			}
			return err
		}

		if err := txn.Delete(modCatKey(userID, module, id)); err != nil {
			return err
		}

		switch module {
		case "contracts":
			return s.contracts.CategoryCascade(txn, userID, id)
		case "purchases":
			return PurchaseCategoryCascade(txn, userID, id)
		}
		return nil
	})
}

// PurchaseCategoryCascade deletes all purchases in a category via the
// purchase index, inside the given transaction.
func PurchaseCategoryCascade(txn *badger.Txn, userID string, categoryID uuid.UUID) error {
	prefix := idxCatPurPrefix(userID, categoryID)
	it := txn.NewIterator(badger.DefaultIteratorOptions)

	var purchaseIDs []uuid.UUID
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		key := it.Item().Key()
		pID, err := uuid.Parse(string(key[len(prefix):]))
		if err != nil {
			continue
		}
		purchaseIDs = append(purchaseIDs, pID)
	}
	it.Close()

	for _, pID := range purchaseIDs {
		if err := txn.Delete(purKey(userID, pID)); err != nil {
			return err
		}
		if err := txn.Delete(idxCatPurKey(userID, categoryID, pID)); err != nil {
			return err
		}
	}
	return nil
}

// Purchases

func (s *BadgerStore) ListPurchases(_ context.Context, userID string) ([]model.Purchase, error) {
	var purchases []model.Purchase
	prefix := purPrefix(userID)

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var p model.Purchase
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &p)
			}); err != nil {
				return err
			}
			purchases = append(purchases, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if purchases == nil {
		purchases = []model.Purchase{}
	}
	for i := range purchases {
		purchases[i] = normalizePurchase(purchases[i])
	}
	return purchases, nil
}

func (s *BadgerStore) ListPurchasesByCategory(_ context.Context, userID string, categoryID uuid.UUID) ([]model.Purchase, error) {
	var purchases []model.Purchase
	prefix := idxCatPurPrefix(userID, categoryID)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().Key()
			pIDStr := string(key[len(prefix):])
			pID, err := uuid.Parse(pIDStr)
			if err != nil {
				continue
			}

			item, err := txn.Get(purKey(userID, pID))
			if err != nil {
				if errors.Is(err, badger.ErrKeyNotFound) {
					continue
				}
				return err
			}

			var p model.Purchase
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &p)
			}); err != nil {
				return err
			}
			purchases = append(purchases, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if purchases == nil {
		purchases = []model.Purchase{}
	}
	for i := range purchases {
		purchases[i] = normalizePurchase(purchases[i])
	}
	return purchases, nil
}

func (s *BadgerStore) GetPurchase(_ context.Context, userID string, id uuid.UUID) (model.Purchase, error) {
	var p model.Purchase
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(purKey(userID, id))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &p)
		})
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return p, ErrNotFound
	}
	return normalizePurchase(p), err
}

func (s *BadgerStore) CreatePurchase(_ context.Context, userID string, p model.Purchase) error {
	p = normalizePurchase(p)
	return s.db.Update(func(txn *badger.Txn) error {
		if err := storePurchase(txn, userID, p); err != nil {
			return err
		}
		return txn.Set(idxCatPurKey(userID, p.CategoryID, p.ID), []byte{})
	})
}

func (s *BadgerStore) UpdatePurchase(_ context.Context, userID string, p model.Purchase) error {
	p = normalizePurchase(p)
	return s.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(purKey(userID, p.ID))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrNotFound
			}
			return err
		}

		var old model.Purchase
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &old)
		}); err != nil {
			return err
		}

		if err := storePurchase(txn, userID, p); err != nil {
			return err
		}

		if old.CategoryID != p.CategoryID {
			if err := txn.Delete(idxCatPurKey(userID, old.CategoryID, p.ID)); err != nil {
				return err
			}
			if err := txn.Set(idxCatPurKey(userID, p.CategoryID, p.ID), []byte{}); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *BadgerStore) DeletePurchase(_ context.Context, userID string, id uuid.UUID) error {
	return s.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(purKey(userID, id))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrNotFound
			}
			return err
		}

		var p model.Purchase
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &p)
		}); err != nil {
			return err
		}
		p = normalizePurchase(p)
		if err := s.RemoveReferences(txn, userID, p.LinkedTransactionIDs, model.LedgerReferencePurchase, id); err != nil {
			return err
		}

		if err := txn.Delete(purKey(userID, id)); err != nil {
			return err
		}
		return txn.Delete(idxCatPurKey(userID, p.CategoryID, id))
	})
}


// Contract and vehicle methods delegate to the module stores until the
// remaining god-interface users (export/restore) move as well.

func (s *BadgerStore) ListContracts(ctx context.Context, userID string) ([]model.Contract, error) {
	return s.contracts.List(ctx, userID)
}

func (s *BadgerStore) ListContractsByCategory(ctx context.Context, userID string, categoryID uuid.UUID) ([]model.Contract, error) {
	return s.contracts.ListByCategory(ctx, userID, categoryID)
}

func (s *BadgerStore) GetContract(ctx context.Context, userID string, id uuid.UUID) (model.Contract, error) {
	return s.contracts.Get(ctx, userID, id)
}

func (s *BadgerStore) CreateContract(ctx context.Context, userID string, c model.Contract) error {
	return s.contracts.Create(ctx, userID, c)
}

func (s *BadgerStore) UpdateContract(ctx context.Context, userID string, c model.Contract) error {
	return s.contracts.Update(ctx, userID, c)
}

func (s *BadgerStore) DeleteContract(ctx context.Context, userID string, id uuid.UUID) error {
	return s.contracts.Delete(ctx, userID, id)
}

func (s *BadgerStore) ListVehicles(ctx context.Context, userID string) ([]model.Vehicle, error) {
	return s.auto.ListVehicles(ctx, userID)
}

func (s *BadgerStore) GetVehicle(ctx context.Context, userID string, id uuid.UUID) (model.Vehicle, error) {
	return s.auto.GetVehicle(ctx, userID, id)
}

func (s *BadgerStore) CreateVehicle(ctx context.Context, userID string, v model.Vehicle) error {
	return s.auto.CreateVehicle(ctx, userID, v)
}

func (s *BadgerStore) UpdateVehicle(ctx context.Context, userID string, v model.Vehicle) error {
	return s.auto.UpdateVehicle(ctx, userID, v)
}

func (s *BadgerStore) DeleteVehicle(ctx context.Context, userID string, id uuid.UUID) error {
	return s.auto.DeleteVehicle(ctx, userID, id)
}

func (s *BadgerStore) ListCostEntries(ctx context.Context, userID string, vehicleID uuid.UUID) ([]model.CostEntry, error) {
	return s.auto.ListCostEntries(ctx, userID, vehicleID)
}

func (s *BadgerStore) GetCostEntry(ctx context.Context, userID string, id uuid.UUID) (model.CostEntry, error) {
	return s.auto.GetCostEntry(ctx, userID, id)
}

func (s *BadgerStore) CreateCostEntry(ctx context.Context, userID string, c model.CostEntry) error {
	return s.auto.CreateCostEntry(ctx, userID, c)
}

func (s *BadgerStore) UpdateCostEntry(ctx context.Context, userID string, c model.CostEntry) error {
	return s.auto.UpdateCostEntry(ctx, userID, c)
}

func (s *BadgerStore) DeleteCostEntry(ctx context.Context, userID string, id uuid.UUID) error {
	return s.auto.DeleteCostEntry(ctx, userID, id)
}
