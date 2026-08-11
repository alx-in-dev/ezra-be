package citylink

import (
	"context"

	"github.com/ezra-game/server/pkg/httputil"
)

// SearchMinLen / SearchLimit bound player search.
const (
	SearchMinLen = 2
	SearchLimit  = 20
)

// EventPublisher pushes a realtime event to a player's SSE stream (R4-6).
// Implemented by realtime.Hub.PublishEvent.
type EventPublisher interface {
	PublishEvent(playerID, eventType string, data map[string]any)
}

// store is the persistence surface the service depends on (satisfied by
// *Repository; an interface so the service is unit-testable with a fake).
type store interface {
	SearchPlayers(ctx context.Context, q, excludeID string, limit int) ([]PlayerBrief, error)
	AreLinked(ctx context.Context, a, b string) (bool, error)
	CreateRequest(ctx context.Context, from, to string) (*LinkRequest, error)
	PendingBetween(ctx context.Context, a, b string) (bool, error)
	GetRequest(ctx context.Context, id string) (*LinkRequest, error)
	SetRequestState(ctx context.Context, id, state string) error
	PendingInbox(ctx context.Context, toPlayer string) ([]LinkRequest, error)
	CreateLink(ctx context.Context, a, b string) error
	ListLinks(ctx context.Context, playerID string) ([]CityLink, error)
	RemoveLink(ctx context.Context, id, playerID string) (int64, error)
	LinkedPartnerIDs(ctx context.Context, playerID string) ([]string, error)
}

// Service orchestrates City Link search, the consent flow, and link management.
type Service struct {
	repo   store
	events EventPublisher
}

func NewService(repo store) *Service { return &Service{repo: repo} }

// WithEvents wires live notifications (incoming request / accepted). Optional.
func (s *Service) WithEvents(p EventPublisher) *Service { s.events = p; return s }

// Search finds candidate partners by username (excludes the requester).
func (s *Service) Search(ctx context.Context, requesterID, q string) ([]PlayerBrief, error) {
	if len([]rune(q)) < SearchMinLen {
		return nil, httputil.NewBadRequest("query_too_short", "введите минимум 2 символа")
	}
	return s.repo.SearchPlayers(ctx, q, requesterID, SearchLimit)
}

// Request proposes a City Link from → to.
func (s *Service) Request(ctx context.Context, from, to string) (*LinkRequest, error) {
	if to == "" || from == to {
		return nil, httputil.NewBadRequest("invalid_target", "нельзя связаться с самим собой")
	}
	if linked, err := s.repo.AreLinked(ctx, from, to); err != nil {
		return nil, err
	} else if linked {
		return nil, httputil.NewBadRequest("already_linked", "вы уже связаны с этим игроком")
	}
	if pending, err := s.repo.PendingBetween(ctx, from, to); err != nil {
		return nil, err
	} else if pending {
		return nil, httputil.NewBadRequest("already_pending", "запрос уже отправлен")
	}
	req, err := s.repo.CreateRequest(ctx, from, to)
	if err != nil {
		// Unique partial index race → treat as a duplicate pending request.
		return nil, httputil.NewBadRequest("already_pending", "запрос уже отправлен")
	}
	if s.events != nil {
		s.events.PublishEvent(to, "link_request", map[string]any{"request_id": req.ID, "from": from})
	}
	return req, nil
}

// Inbox returns the player's pending incoming requests.
func (s *Service) Inbox(ctx context.Context, playerID string) ([]LinkRequest, error) {
	return s.repo.PendingInbox(ctx, playerID)
}

// Accept turns a pending request (addressed to playerID) into an active link.
func (s *Service) Accept(ctx context.Context, playerID, reqID string) (*LinkRequest, error) {
	req, err := s.requireRecipientPending(ctx, playerID, reqID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SetRequestState(ctx, reqID, StateAccepted); err != nil {
		return nil, err
	}
	if err := s.repo.CreateLink(ctx, req.FromPlayer, req.ToPlayer); err != nil {
		return nil, err
	}
	if s.events != nil {
		s.events.PublishEvent(req.FromPlayer, "link_accepted", map[string]any{"partner": playerID})
	}
	return req, nil
}

// Reject declines a pending request addressed to playerID.
func (s *Service) Reject(ctx context.Context, playerID, reqID string) error {
	if _, err := s.requireRecipientPending(ctx, playerID, reqID); err != nil {
		return err
	}
	return s.repo.SetRequestState(ctx, reqID, StateRejected)
}

func (s *Service) requireRecipientPending(ctx context.Context, playerID, reqID string) (*LinkRequest, error) {
	req, err := s.repo.GetRequest(ctx, reqID)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, httputil.NewNotFound("not_found", "запрос не найден")
	}
	if req.ToPlayer != playerID {
		return nil, httputil.NewForbidden("not_recipient", "это не ваш запрос")
	}
	if req.State != StatePending {
		return nil, httputil.NewBadRequest("not_pending", "запрос уже обработан")
	}
	return req, nil
}

// Links lists the player's active City Links.
func (s *Service) Links(ctx context.Context, playerID string) ([]CityLink, error) {
	return s.repo.ListLinks(ctx, playerID)
}

// Remove dissolves a link the player belongs to.
func (s *Service) Remove(ctx context.Context, playerID, linkID string) error {
	affected, err := s.repo.RemoveLink(ctx, linkID, playerID)
	if err != nil {
		return err
	}
	if affected == 0 {
		return httputil.NewNotFound("not_found", "связь не найдена")
	}
	return nil
}

// AreLinked reports whether two players are City-Linked (used by mutual-support
// features in slice 2).
func (s *Service) AreLinked(ctx context.Context, a, b string) (bool, error) {
	return s.repo.AreLinked(ctx, a, b)
}

// LinkedPartnerIDs returns the player's linked partners (used by alert fan-out).
func (s *Service) LinkedPartnerIDs(ctx context.Context, playerID string) ([]string, error) {
	return s.repo.LinkedPartnerIDs(ctx, playerID)
}
