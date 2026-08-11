// Package citylink implements R4-4b City Link: a mutual-support alliance between
// two players (repair each other's beacons incl. legacy, see each other's
// defense alerts). No network/Ecap/income merge — purely co-op support.
package citylink

import "time"

// PlayerBrief is a minimal player card for search results / partners.
type PlayerBrief struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Level    int    `json:"level"`
}

// LinkRequest is a pending/decided consent request from one player to another.
type LinkRequest struct {
	ID           string    `json:"id"`
	FromPlayer   string    `json:"from_player"`
	ToPlayer     string    `json:"to_player"`
	State        string    `json:"state"` // pending | accepted | rejected | cancelled
	CreatedAt    time.Time `json:"created_at"`
	FromUsername string    `json:"from_username,omitempty"` // joined for inbox display
}

// CityLink is an active alliance as seen by one of its members (partner side).
type CityLink struct {
	ID              string    `json:"id"`
	PartnerID       string    `json:"partner_id"`
	PartnerUsername string    `json:"partner_username"`
	CreatedAt       time.Time `json:"created_at"`
}

// Request states.
const (
	StatePending   = "pending"
	StateAccepted  = "accepted"
	StateRejected  = "rejected"
	StateCancelled = "cancelled"
)
