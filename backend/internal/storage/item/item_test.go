package item

import (
	"errors"
	"log/slog"
	"slices"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

const user = "u1"

type thing struct {
	ID       uuid.UUID   `json:"id"`
	ParentID uuid.UUID   `json:"parentId"`
	Name     string      `json:"name"`
	Links    []uuid.UUID `json:"links,omitempty"`
}

func thingSpec(index string) Spec[thing] {
	spec := Spec[thing]{
		Module: "mod", Kind: "thg", RefType: "thing",
		ID:    func(t thing) uuid.UUID { return t.ID },
		Links: func(t *thing) *[]uuid.UUID { return &t.Links },
	}
	if index != "" {
		spec.Index = index
		spec.Parent = func(t thing) uuid.UUID { return t.ParentID }
	}
	return spec
}

type removal struct {
	ids      []uuid.UUID
	refType  string
	targetID uuid.UUID
}

type fakeSide struct {
	removed  []removal
	existing map[uuid.UUID]bool
	fail     error
}

func (f *fakeSide) RemoveReferences(_ *badger.Txn, _ string, ids []uuid.UUID, refType string, targetID uuid.UUID) error {
	if f.fail != nil {
		return f.fail
	}
	f.removed = append(f.removed, removal{slices.Clone(ids), refType, targetID})
	return nil
}

func (f *fakeSide) TransactionExists(_ *badger.Txn, _ string, id uuid.UUID) bool {
	return f.existing[id]
}

type env struct {
	e     *storage.Engine
	links *link.Registry
	side  *fakeSide
	s     *Store[thing]
}

func newEnv(t *testing.T, index string) env {
	t.Helper()
	e, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	links := link.NewRegistry()
	side := &fakeSide{existing: map[uuid.UUID]bool{}}
	links.SetTransactionSide(side)
	return env{e: e, links: links, side: side, s: New(e, links, thingSpec(index))}
}

func (v env) create(t *testing.T, parent uuid.UUID, name string, links ...uuid.UUID) thing {
	t.Helper()
	th := thing{ID: uuid.New(), ParentID: parent, Name: name, Links: links}
	if err := v.s.Create(t.Context(), user, th); err != nil {
		t.Fatal(err)
	}
	return th
}

func (v env) hasKey(t *testing.T, key string) bool {
	t.Helper()
	found := false
	if err := v.e.View(func(txn *badger.Txn) error {
		_, err := txn.Get([]byte(key))
		found = err == nil
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return found
}

func (v env) set(t *testing.T, key, val string) {
	t.Helper()
	if err := v.e.Update(func(txn *badger.Txn) error { return txn.Set([]byte(key), []byte(val)) }); err != nil {
		t.Fatal(err)
	}
}

func (v env) linksOf(t *testing.T, id uuid.UUID) []uuid.UUID {
	t.Helper()
	th, err := v.s.Get(t.Context(), user, id)
	if err != nil {
		t.Fatal(err)
	}
	return th.Links
}

func TestNew_RegistersTargetAndChecksSpec(t *testing.T) {
	v := newEnv(t, "p_thg")
	if got, err := v.links.Target("thing"); err != nil || got != link.Target(v.s) {
		t.Errorf("registered target = %v, %v", got, err)
	}
	for name, spec := range map[string]Spec[thing]{
		"no module":        {Kind: "k", RefType: "r", ID: thingSpec("").ID, Links: thingSpec("").Links},
		"no links":         {Module: "m", Kind: "k", RefType: "r", ID: thingSpec("").ID},
		"index w/o parent": {Module: "m", Kind: "k", RefType: "r", Index: "i", ID: thingSpec("").ID, Links: thingSpec("").Links},
		"parent w/o index": {Module: "m", Kind: "k", RefType: "r", ID: thingSpec("").ID, Links: thingSpec("").Links, Parent: thingSpec("i").Parent},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("New accepted an incomplete spec")
				}
			}()
			New(v.e, v.links, spec)
		})
	}
}

func TestKeys(t *testing.T) {
	v := newEnv(t, "p_thg")
	parent := uuid.New()
	th := v.create(t, parent, "A")
	for _, key := range []string{
		"u/u1/mod/mod/thg/" + th.ID.String(),
		"u/u1/mod/mod/idx/p_thg/" + parent.String() + "/" + th.ID.String(),
	} {
		if !v.hasKey(t, key) {
			t.Errorf("key %q missing", key)
		}
	}

	flat := newEnv(t, "")
	th = flat.create(t, parent, "B")
	if !flat.hasKey(t, "u/u1/mod/mod/thg/"+th.ID.String()) {
		t.Error("item key missing without index")
	}
	if flat.hasKey(t, "u/u1/mod/mod/idx/p_thg/"+parent.String()+"/"+th.ID.String()) {
		t.Error("index key written without index")
	}
}

func TestCreateGetList(t *testing.T) {
	v := newEnv(t, "p_thg")
	ctx := t.Context()
	if list, err := v.s.List(ctx, user); err != nil || list == nil || len(list) != 0 {
		t.Fatalf("List empty = %v, %v", list, err)
	}
	if list, err := v.s.ListByParent(ctx, user, uuid.New()); err != nil || list == nil || len(list) != 0 {
		t.Fatalf("ListByParent empty = %v, %v", list, err)
	}

	p1, p2, txnID := uuid.New(), uuid.New(), uuid.New()
	a := v.create(t, p1, "A", txnID, uuid.Nil, txnID)
	v.create(t, p1, "B")
	v.create(t, p2, "C")

	got, err := v.s.Get(ctx, user, a.ID)
	if err != nil || got.Name != "A" || !slices.Equal(got.Links, []uuid.UUID{txnID}) {
		t.Errorf("Get = %+v, %v; want links normalized", got, err)
	}
	if _, err := v.s.Get(ctx, user, uuid.New()); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("Get unknown = %v", err)
	}
	if list, _ := v.s.List(ctx, user); len(list) != 3 {
		t.Errorf("List = %d items", len(list))
	}
	if list, _ := v.s.List(ctx, "u2"); len(list) != 0 {
		t.Errorf("List of another user = %d items", len(list))
	}

	v.set(t, "u/u1/mod/mod/idx/p_thg/"+p1.String()+"/"+uuid.New().String(), "")
	v.set(t, "u/u1/mod/mod/idx/p_thg/"+p1.String()+"/junk", "")
	list, err := v.s.ListByParent(ctx, user, p1)
	if err != nil || len(list) != 2 {
		t.Errorf("ListByParent = %+v, %v; want A and B", list, err)
	}
}

func TestCorruptRecords(t *testing.T) {
	v := newEnv(t, "p_thg")
	ctx := t.Context()
	parent, bad := uuid.New(), uuid.New()
	v.set(t, "u/u1/mod/mod/thg/"+bad.String(), "{")
	v.set(t, "u/u1/mod/mod/idx/p_thg/"+parent.String()+"/"+bad.String(), "")

	if _, err := v.s.List(ctx, user); err == nil {
		t.Error("List ignored a corrupt record")
	}
	if _, err := v.s.ListByParent(ctx, user, parent); err == nil {
		t.Error("ListByParent ignored a corrupt record")
	}
	if _, err := v.s.PruneDeadLinks(ctx, user); err == nil {
		t.Error("PruneDeadLinks ignored a corrupt record")
	}
	if err := v.e.Update(func(txn *badger.Txn) error { return v.s.DeleteByParentTxn(txn, user, parent) }); err == nil {
		t.Error("DeleteByParentTxn ignored a corrupt record")
	}
	if err := v.e.Update(func(txn *badger.Txn) error { return v.s.RemoveLink(txn, user, bad, uuid.New()) }); err == nil {
		t.Error("RemoveLink ignored a corrupt record")
	}
}

func TestUpdate_KeepsStoredLinks(t *testing.T) {
	v := newEnv(t, "p_thg")
	ctx := t.Context()
	th := v.create(t, uuid.New(), "A")
	stale, _ := v.s.Get(ctx, user, th.ID)

	txnID := uuid.New()
	if err := v.e.Update(func(txn *badger.Txn) error { return v.s.AddLink(txn, user, th.ID, txnID) }); err != nil {
		t.Fatal(err)
	}
	stale.Name = "A2"
	stale.Links = []uuid.UUID{uuid.New()}
	if err := v.s.Update(ctx, user, stale); err != nil {
		t.Fatal(err)
	}
	got, _ := v.s.Get(ctx, user, th.ID)
	if got.Name != "A2" || !slices.Equal(got.Links, []uuid.UUID{txnID}) {
		t.Errorf("after update = %+v, want name A2 and the stored link [%s]", got, txnID)
	}

	if err := v.s.Update(ctx, user, thing{ID: uuid.New()}); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("Update unknown = %v", err)
	}
}

func TestUpdate_MovesIndex(t *testing.T) {
	v := newEnv(t, "p_thg")
	ctx := t.Context()
	p1, p2 := uuid.New(), uuid.New()
	th := v.create(t, p1, "A")

	th.Name = "same parent"
	if err := v.s.Update(ctx, user, th); err != nil {
		t.Fatal(err)
	}
	th.ParentID = p2
	if err := v.s.Update(ctx, user, th); err != nil {
		t.Fatal(err)
	}
	if l, _ := v.s.ListByParent(ctx, user, p1); len(l) != 0 {
		t.Errorf("old parent lists %d", len(l))
	}
	if l, _ := v.s.ListByParent(ctx, user, p2); len(l) != 1 || l[0].Name != "same parent" {
		t.Errorf("new parent lists %+v", l)
	}

	flat := newEnv(t, "")
	f := flat.create(t, p1, "F")
	f.ParentID = p2
	if err := flat.s.Update(ctx, user, f); err != nil {
		t.Fatal(err)
	}
}

func TestDelete(t *testing.T) {
	for _, index := range []string{"p_thg", ""} {
		t.Run("index="+index, func(t *testing.T) {
			v := newEnv(t, index)
			ctx := t.Context()
			parent, txnID := uuid.New(), uuid.New()
			th := v.create(t, parent, "A", txnID)

			if err := v.s.Delete(ctx, user, th.ID); err != nil {
				t.Fatal(err)
			}
			if v.hasKey(t, "u/u1/mod/mod/thg/"+th.ID.String()) || v.hasKey(t, "u/u1/mod/mod/idx/p_thg/"+parent.String()+"/"+th.ID.String()) {
				t.Error("item or index key survived")
			}
			want := []removal{{[]uuid.UUID{txnID}, "thing", th.ID}}
			if len(v.side.removed) != 1 || !slices.Equal(v.side.removed[0].ids, want[0].ids) || v.side.removed[0].targetID != th.ID || v.side.removed[0].refType != "thing" {
				t.Errorf("removed = %+v, want %+v", v.side.removed, want)
			}
			if err := v.s.Delete(ctx, user, th.ID); !errors.Is(err, storage.ErrNotFound) {
				t.Errorf("second Delete = %v", err)
			}
		})
	}
}

func TestDelete_ReferenceFailureKeepsItem(t *testing.T) {
	v := newEnv(t, "p_thg")
	th := v.create(t, uuid.New(), "A")
	v.side.fail = errors.New("ledger down")
	if err := v.s.Delete(t.Context(), user, th.ID); err == nil {
		t.Fatal("Delete succeeded")
	}
	if _, err := v.s.Get(t.Context(), user, th.ID); err != nil {
		t.Errorf("item gone: %v", err)
	}
}

func TestDeleteByParentTxn(t *testing.T) {
	v := newEnv(t, "p_thg")
	ctx := t.Context()
	parent, other := uuid.New(), uuid.New()
	t1, t2 := uuid.New(), uuid.New()
	a := v.create(t, parent, "A", t1)
	b := v.create(t, parent, "B", t2, t1)
	kept := v.create(t, other, "C", t1)
	dangling := uuid.New()
	v.set(t, "u/u1/mod/mod/idx/p_thg/"+parent.String()+"/"+dangling.String(), "")

	if err := v.e.Update(func(txn *badger.Txn) error { return v.s.DeleteByParentTxn(txn, user, parent) }); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{a.ID, b.ID, dangling} {
		if v.hasKey(t, "u/u1/mod/mod/thg/"+id.String()) || v.hasKey(t, "u/u1/mod/mod/idx/p_thg/"+parent.String()+"/"+id.String()) {
			t.Errorf("%s survived the cascade", id)
		}
	}
	if _, err := v.s.Get(ctx, user, kept.ID); err != nil {
		t.Errorf("item of another parent: %v", err)
	}
	wantRefs := map[uuid.UUID][]uuid.UUID{a.ID: {t1}, b.ID: {t2, t1}}
	if len(v.side.removed) != len(wantRefs) {
		t.Fatalf("removed = %+v", v.side.removed)
	}
	for _, r := range v.side.removed {
		if r.refType != "thing" || !slices.Equal(r.ids, wantRefs[r.targetID]) {
			t.Errorf("removal %+v, want %v", r, wantRefs[r.targetID])
		}
	}

	c := v.create(t, other, "D")
	v.side.fail = errors.New("ledger down")
	if err := v.e.Update(func(txn *badger.Txn) error { return v.s.DeleteByParentTxn(txn, user, other) }); err == nil {
		t.Error("cascade succeeded although the reference removal failed")
	}
	if _, err := v.s.Get(ctx, user, c.ID); err != nil {
		t.Errorf("item gone after failed cascade: %v", err)
	}
}

func TestLinkTarget(t *testing.T) {
	v := newEnv(t, "")
	th := v.create(t, uuid.New(), "A")
	txnID := uuid.New()
	update := func(fn func(txn *badger.Txn) error) error { return v.e.Update(fn) }

	for range 2 {
		if err := update(func(txn *badger.Txn) error { return v.s.AddLink(txn, user, th.ID, txnID) }); err != nil {
			t.Fatal(err)
		}
	}
	if got := v.linksOf(t, th.ID); !slices.Equal(got, []uuid.UUID{txnID}) {
		t.Errorf("links after AddLink twice = %v", got)
	}
	if err := update(func(txn *badger.Txn) error { return v.s.AddLink(txn, user, uuid.New(), txnID) }); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("AddLink unknown = %v", err)
	}
	if err := update(func(txn *badger.Txn) error { return v.s.RemoveLink(txn, user, th.ID, txnID) }); err != nil {
		t.Fatal(err)
	}
	if got := v.linksOf(t, th.ID); len(got) != 0 {
		t.Errorf("links after RemoveLink = %v", got)
	}
	if err := update(func(txn *badger.Txn) error { return v.s.RemoveLink(txn, user, uuid.New(), txnID) }); err != nil {
		t.Errorf("RemoveLink unknown = %v", err)
	}
	if err := v.e.View(func(txn *badger.Txn) error {
		if !v.s.Exists(txn, user, th.ID) || v.s.Exists(txn, user, uuid.New()) {
			t.Error("Exists wrong")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPruneDeadLinks(t *testing.T) {
	v := newEnv(t, "p_thg")
	ctx := t.Context()
	alive, dead1, dead2 := uuid.New(), uuid.New(), uuid.New()
	v.side.existing[alive] = true
	a := v.create(t, uuid.New(), "A", dead1, alive, dead2)
	b := v.create(t, uuid.New(), "B", alive)
	c := v.create(t, uuid.New(), "C")

	pruned, err := v.s.PruneDeadLinks(ctx, user)
	if err != nil || pruned != 2 {
		t.Fatalf("pruned = %d, %v; want 2", pruned, err)
	}
	for id, want := range map[uuid.UUID][]uuid.UUID{a.ID: {alive}, b.ID: {alive}, c.ID: {}} {
		if got := v.linksOf(t, id); !slices.Equal(got, want) {
			t.Errorf("links of %s = %v, want %v", id, got, want)
		}
	}
	if pruned, err := v.s.PruneDeadLinks(ctx, user); err != nil || pruned != 0 {
		t.Errorf("second prune = %d, %v", pruned, err)
	}
}
