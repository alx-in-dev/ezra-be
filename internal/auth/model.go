package auth

// LoginRequest accepts either a Firebase token or legacy login/password credentials.
type LoginRequest struct {
	FirebaseToken string `json:"firebase_token"`
	Login         string `json:"login"`
	Password      string `json:"password"`
	Username      string `json:"username,omitempty"`
}

// RegisterRequest accepts either a Firebase token or legacy login/password credentials.
type RegisterRequest struct {
	FirebaseToken string `json:"firebase_token"`
	Login         string `json:"login"`
	Password      string `json:"password"`
	Username      string `json:"username,omitempty"`
}

// AuthResponse is returned by both login and register endpoints.
type AuthResponse struct {
	Player       any    `json:"player"`
	SessionToken string `json:"session_token"`
}
