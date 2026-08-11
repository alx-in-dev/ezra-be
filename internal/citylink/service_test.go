package citylink

import (
	"context"
	"testing"

	"github.com/ezra-game/server/pkg/httputil"
)

// fakeStore is an in-memory store for service tests.
type fakeStore struct {
	linked    map[string]bool // canonical "lo|hi" → linked
	pending   map[string]bool // "from|to" → pending
	requests  map[string]*LinkRequest
	createdLk [][2]string
	nextID    int
}

func newFakeStore() *fakeStore {
	return &fakeStore{linked: map[string]bool{}, pending: map[string]bool{}, requests: map[string]*LinkRequest{}}
}

func key(a, b string) string {
	lo, hi := canonical(a, b)
	return lo + "|" + hi
}

func (f *fakeStore) SearchPlayers(_ context.Context, q, ex string, _ int) ([]PlayerBrief, error) {
	return []PlayerBrief{{ID: "p2", Username: "neo", Level: 3}}, nil
}
func (f *fakeStore) AreLinked(_ context.Context, a, b string) (bool, error) { return f.linked[key(a, b)], nil }
func (f *fakeStore) PendingBetween(_ context.Context, a, b string) (bool, error) {
	return f.pending[a+"|"+b] || f.pending[b+"|"+a], nil
}
func (f *fakeStore) CreateRequest(_ context.Context, from, to string) (*LinkRequest, error) {
	f.nextID++
	id := string(rune('a' + f.nextID))
	req := &LinkRequest{ID: id, FromPlayer: from, ToPlayer: to, State: StatePending}
	f.requests[id] = req
	f.pending[from+"|"+to] = true
	return req, nil
}
func (f *fakeStore) GetRequest(_ context.Context, id string) (*LinkRequest, error) {
	return f.requests[id], nil
}
func (f *fakeStore) SetRequestState(_ context.Context, id, state string) error {
	if r := f.requests[id]; r != nil {
		r.State = state
	}
	return nil
}
func (f *fakeStore) PendingInbox(_ context.Context, to string) ([]LinkRequest, error) { return nil, nil }
func (f *fakeStore) CreateLink(_ context.Context, a, b string) error {
	f.linked[key(a, b)] = true
	f.createdLk = append(f.createdLk, [2]string{a, b})
	return nil
}
func (f *fakeStore) ListLinks(_ context.Context, _ string) ([]CityLink, error) { return nil, nil }
func (f *fakeStore) RemoveLink(_ context.Context, _, _ string) (int64, error)  { return 1, nil }
func (f *fakeStore) LinkedPartnerIDs(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func code(t *testing.T, err error) string {
	t.Helper()
	if ae, ok := err.(*httputil.AppError); ok {
		return ae.Code
	}
	t.Fatalf("want AppError, got %v", err)
	return ""
}

func TestRequest_Guards(t *testing.T) {
	ctx := context.Background()
	st := newFakeStore()
	svc := NewService(st)

	// self
	if _, err := svc.Request(ctx, "p1", "p1"); code(t, err) != "invalid_target" {
		t.Fatal("self-link should be rejected")
	}
	// already linked
	st.linked[key("p1", "p2")] = true
	if _, err := svc.Request(ctx, "p1", "p2"); code(t, err) != "already_linked" {
		t.Fatal("already-linked should be rejected")
	}
	// pending
	st2 := newFakeStore()
	svc2 := NewService(st2)
	st2.pending["p2|p1"] = true
	if _, err := svc2.Request(ctx, "p1", "p2"); code(t, err) != "already_pending" {
		t.Fatal("pending (either direction) should be rejected")
	}
	// success
	st3 := newFakeStore()
	svc3 := NewService(st3)
	req, err := svc3.Request(ctx, "p1", "p2")
	if err != nil || req.State != StatePending {
		t.Fatalf("request should succeed pending: %v %+v", err, req)
	}
}

func TestAccept_CreatesLinkForRecipientOnly(t *testing.T) {
	ctx := context.Background()
	st := newFakeStore()
	svc := NewService(st)
	req, _ := svc.Request(ctx, "p1", "p2") // p1 → p2

	// non-recipient cannot accept
	if err := mustErr(svc.Accept(ctx, "p9", req.ID)); code(t, err) != "not_recipient" {
		t.Fatal("only the recipient may accept")
	}
	// recipient accepts → link created
	if _, err := svc.Accept(ctx, "p2", req.ID); err != nil {
		t.Fatalf("recipient accept failed: %v", err)
	}
	if linked, _ := st.AreLinked(ctx, "p1", "p2"); !linked {
		t.Fatal("link should exist after accept")
	}
	// re-accept now not pending
	if err := mustErr(svc.Accept(ctx, "p2", req.ID)); code(t, err) != "not_pending" {
		t.Fatal("already-handled request should reject re-accept")
	}
}

func mustErr(_ *LinkRequest, err error) error { return err }
