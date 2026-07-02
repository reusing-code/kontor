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
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/migration"
)

var (
	ErrNotFound = storage.ErrNotFound
	ErrConflict = storage.ErrConflict
)

type BackupConfig = storage.BackupConfig

type BadgerStore struct {
	engine *storage.Engine
	db     *badger.DB
	logger *slog.Logger
}

func NewBadgerStore(path string, logger *slog.Logger) (*BadgerStore, error) {
	engine, err := storage.Open(path, logger)
	if err != nil {
		return nil, err
	}

	if err := migration.RunAll(engine.DB(), logger, migration.All); err != nil {
		engine.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return &BadgerStore{
		engine: engine,
		db:     engine.DB(),
		logger: logger,
	}, nil
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

// Contract key helpers

func conKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/con/%s", userID, id))
}

func conPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/con/", userID))
}

func idxCatConKey(userID string, categoryID, contractID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/cat_con/%s/%s", userID, categoryID, contractID))
}

func idxCatConPrefix(userID string, categoryID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/cat_con/%s/", userID, categoryID))
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

		if module == "contracts" {
			// Delete all contracts in this category via the contract index
			conPrefix := idxCatConPrefix(userID, id)
			it := txn.NewIterator(badger.DefaultIteratorOptions)

			var contractIDs []uuid.UUID
			for it.Seek(conPrefix); it.ValidForPrefix(conPrefix); it.Next() {
				key := it.Item().Key()
				conIDStr := string(key[len(conPrefix):])
				conID, err := uuid.Parse(conIDStr)
				if err != nil {
					continue
				}
				contractIDs = append(contractIDs, conID)
			}
			it.Close()

			for _, cID := range contractIDs {
				if err := txn.Delete(conKey(userID, cID)); err != nil {
					return err
				}
				if err := txn.Delete(idxCatConKey(userID, id, cID)); err != nil {
					return err
				}
			}
		}

		if module == "purchases" {
			// Delete all purchases in this category via the purchase index
			purIdxPrefix := idxCatPurPrefix(userID, id)
			it2 := txn.NewIterator(badger.DefaultIteratorOptions)

			var purchaseIDs []uuid.UUID
			for it2.Seek(purIdxPrefix); it2.ValidForPrefix(purIdxPrefix); it2.Next() {
				key := it2.Item().Key()
				pIDStr := string(key[len(purIdxPrefix):])
				pID, err := uuid.Parse(pIDStr)
				if err != nil {
					continue
				}
				purchaseIDs = append(purchaseIDs, pID)
			}
			it2.Close()

			for _, pID := range purchaseIDs {
				if err := txn.Delete(purKey(userID, pID)); err != nil {
					return err
				}
				if err := txn.Delete(idxCatPurKey(userID, id, pID)); err != nil {
					return err
				}
			}
		}

		return nil
	})
}

// Contracts

func (s *BadgerStore) ListContracts(_ context.Context, userID string) ([]model.Contract, error) {
	var contracts []model.Contract
	prefix := conPrefix(userID)

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var con model.Contract
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &con)
			}); err != nil {
				return err
			}
			contracts = append(contracts, con)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if contracts == nil {
		contracts = []model.Contract{}
	}
	for i := range contracts {
		contracts[i] = normalizeContract(contracts[i])
	}
	return contracts, nil
}

func (s *BadgerStore) ListContractsByCategory(_ context.Context, userID string, categoryID uuid.UUID) ([]model.Contract, error) {
	var contracts []model.Contract
	prefix := idxCatConPrefix(userID, categoryID)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().Key()
			conIDStr := string(key[len(prefix):])
			conID, err := uuid.Parse(conIDStr)
			if err != nil {
				continue
			}

			item, err := txn.Get(conKey(userID, conID))
			if err != nil {
				if errors.Is(err, badger.ErrKeyNotFound) {
					continue
				}
				return err
			}

			var con model.Contract
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &con)
			}); err != nil {
				return err
			}
			contracts = append(contracts, con)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if contracts == nil {
		contracts = []model.Contract{}
	}
	for i := range contracts {
		contracts[i] = normalizeContract(contracts[i])
	}
	return contracts, nil
}

func (s *BadgerStore) GetContract(_ context.Context, userID string, id uuid.UUID) (model.Contract, error) {
	var con model.Contract
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(conKey(userID, id))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &con)
		})
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return con, ErrNotFound
	}
	return normalizeContract(con), err
}

func (s *BadgerStore) CreateContract(_ context.Context, userID string, c model.Contract) error {
	c = normalizeContract(c)
	return s.db.Update(func(txn *badger.Txn) error {
		if err := storeContract(txn, userID, c); err != nil {
			return err
		}
		return txn.Set(idxCatConKey(userID, c.CategoryID, c.ID), []byte{})
	})
}

func (s *BadgerStore) UpdateContract(_ context.Context, userID string, c model.Contract) error {
	c = normalizeContract(c)
	return s.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(conKey(userID, c.ID))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrNotFound
			}
			return err
		}

		var old model.Contract
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &old)
		}); err != nil {
			return err
		}

		if err := storeContract(txn, userID, c); err != nil {
			return err
		}

		if old.CategoryID != c.CategoryID {
			if err := txn.Delete(idxCatConKey(userID, old.CategoryID, c.ID)); err != nil {
				return err
			}
			if err := txn.Set(idxCatConKey(userID, c.CategoryID, c.ID), []byte{}); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *BadgerStore) DeleteContract(_ context.Context, userID string, id uuid.UUID) error {
	return s.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(conKey(userID, id))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrNotFound
			}
			return err
		}

		var con model.Contract
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &con)
		}); err != nil {
			return err
		}
		con = normalizeContract(con)
		if err := removeLedgerTransactionLinks(txn, userID, con.LinkedTransactionIDs, model.LedgerReferenceContract, id); err != nil {
			return err
		}

		if err := txn.Delete(conKey(userID, id)); err != nil {
			return err
		}
		return txn.Delete(idxCatConKey(userID, con.CategoryID, id))
	})
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
		if err := removeLedgerTransactionLinks(txn, userID, p.LinkedTransactionIDs, model.LedgerReferencePurchase, id); err != nil {
			return err
		}

		if err := txn.Delete(purKey(userID, id)); err != nil {
			return err
		}
		return txn.Delete(idxCatPurKey(userID, p.CategoryID, id))
	})
}

// Vehicle key helpers
// Key format: u/{userID}/veh/{vehicleID}

func vehKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/veh/%s", userID, id))
}

func vehPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/veh/", userID))
}

// Cost entry key helpers
// Key format: u/{userID}/cost/{costEntryID}
// Index: u/{userID}/idx/veh_cost/{vehicleID}/{costEntryID}

func costKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/cost/%s", userID, id))
}

func idxVehCostKey(userID string, vehicleID, costID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/veh_cost/%s/%s", userID, vehicleID, costID))
}

func idxVehCostPrefix(userID string, vehicleID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/veh_cost/%s/", userID, vehicleID))
}

// Vehicles

func (s *BadgerStore) ListVehicles(_ context.Context, userID string) ([]model.Vehicle, error) {
	var vehicles []model.Vehicle
	prefix := vehPrefix(userID)

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var v model.Vehicle
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &v)
			}); err != nil {
				return err
			}
			vehicles = append(vehicles, v)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if vehicles == nil {
		vehicles = []model.Vehicle{}
	}
	for i := range vehicles {
		vehicles[i] = normalizeVehicle(vehicles[i])
	}
	return vehicles, nil
}

func (s *BadgerStore) GetVehicle(_ context.Context, userID string, id uuid.UUID) (model.Vehicle, error) {
	var v model.Vehicle
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(vehKey(userID, id))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &v)
		})
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return v, ErrNotFound
	}
	return normalizeVehicle(v), err
}

func (s *BadgerStore) CreateVehicle(_ context.Context, userID string, v model.Vehicle) error {
	v = normalizeVehicle(v)
	return s.db.Update(func(txn *badger.Txn) error {
		return storeVehicle(txn, userID, v)
	})
}

func (s *BadgerStore) UpdateVehicle(_ context.Context, userID string, v model.Vehicle) error {
	v = normalizeVehicle(v)
	return s.db.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(vehKey(userID, v.ID)); err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrNotFound
			}
			return err
		}
		return storeVehicle(txn, userID, v)
	})
}

func (s *BadgerStore) DeleteVehicle(_ context.Context, userID string, id uuid.UUID) error {
	return s.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(vehKey(userID, id))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrNotFound
			}
			return err
		}

		var vehicle model.Vehicle
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &vehicle)
		}); err != nil {
			return err
		}
		vehicle = normalizeVehicle(vehicle)
		if err := removeLedgerTransactionLinks(txn, userID, vehicle.LinkedTransactionIDs, model.LedgerReferenceVehicle, id); err != nil {
			return err
		}

		if err := txn.Delete(vehKey(userID, id)); err != nil {
			return err
		}

		// Cascade delete all cost entries for this vehicle
		idxPrefix := idxVehCostPrefix(userID, id)
		it := txn.NewIterator(badger.DefaultIteratorOptions)

		var costIDs []uuid.UUID
		for it.Seek(idxPrefix); it.ValidForPrefix(idxPrefix); it.Next() {
			key := it.Item().Key()
			cIDStr := string(key[len(idxPrefix):])
			cID, err := uuid.Parse(cIDStr)
			if err != nil {
				continue
			}
			costIDs = append(costIDs, cID)
		}
		it.Close()

		for _, cID := range costIDs {
			if err := txn.Delete(costKey(userID, cID)); err != nil {
				return err
			}
			if err := txn.Delete(idxVehCostKey(userID, id, cID)); err != nil {
				return err
			}
		}

		return nil
	})
}

// Cost Entries

func (s *BadgerStore) ListCostEntries(_ context.Context, userID string, vehicleID uuid.UUID) ([]model.CostEntry, error) {
	var entries []model.CostEntry
	prefix := idxVehCostPrefix(userID, vehicleID)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().Key()
			cIDStr := string(key[len(prefix):])
			cID, err := uuid.Parse(cIDStr)
			if err != nil {
				continue
			}

			item, err := txn.Get(costKey(userID, cID))
			if err != nil {
				if errors.Is(err, badger.ErrKeyNotFound) {
					continue
				}
				return err
			}

			var c model.CostEntry
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &c)
			}); err != nil {
				return err
			}
			entries = append(entries, c)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []model.CostEntry{}
	}
	return entries, nil
}

func (s *BadgerStore) GetCostEntry(_ context.Context, userID string, id uuid.UUID) (model.CostEntry, error) {
	var c model.CostEntry
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(costKey(userID, id))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &c)
		})
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return c, ErrNotFound
	}
	return c, err
}

func (s *BadgerStore) CreateCostEntry(_ context.Context, userID string, c model.CostEntry) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return s.db.Update(func(txn *badger.Txn) error {
		if err := txn.Set(costKey(userID, c.ID), data); err != nil {
			return err
		}
		return txn.Set(idxVehCostKey(userID, c.VehicleID, c.ID), []byte{})
	})
}

func (s *BadgerStore) UpdateCostEntry(_ context.Context, userID string, c model.CostEntry) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return s.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(costKey(userID, c.ID))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrNotFound
			}
			return err
		}

		var old model.CostEntry
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &old)
		}); err != nil {
			return err
		}

		if err := txn.Set(costKey(userID, c.ID), data); err != nil {
			return err
		}

		if old.VehicleID != c.VehicleID {
			if err := txn.Delete(idxVehCostKey(userID, old.VehicleID, c.ID)); err != nil {
				return err
			}
			if err := txn.Set(idxVehCostKey(userID, c.VehicleID, c.ID), []byte{}); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *BadgerStore) DeleteCostEntry(_ context.Context, userID string, id uuid.UUID) error {
	return s.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(costKey(userID, id))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrNotFound
			}
			return err
		}

		var c model.CostEntry
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &c)
		}); err != nil {
			return err
		}

		if err := txn.Delete(costKey(userID, id)); err != nil {
			return err
		}
		return txn.Delete(idxVehCostKey(userID, c.VehicleID, id))
	})
}
