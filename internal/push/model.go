package push

// PushToken stores a player's FCM registration token.
type PushToken struct {
	PlayerID string `json:"player_id" db:"player_id"`
	FCMToken string `json:"fcm_token" db:"fcm_token"`
	Platform string `json:"platform" db:"platform"`
}

// PushMessage describes a push notification to be sent.
type PushMessage struct {
	PlayerID string
	Type     string // tower_attacked, pet_returned, rift_nearby, army_decay, event, streak_expiring
	Title    string
	Body     string
	Data     map[string]string
}
