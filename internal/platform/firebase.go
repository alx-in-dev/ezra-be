package platform

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

// ErrFCMTokenNotRegistered marks a device token FCM reports as stale;
// callers should drop the token and not retry.
var ErrFCMTokenNotRegistered = errors.New("fcm token not registered")

// FirebaseAuth wraps the Firebase Admin SDK auth client.
type FirebaseAuth struct {
	client *auth.Client
}

// FirebaseMessaging wraps the Firebase Admin SDK FCM client.
type FirebaseMessaging struct {
	client *messaging.Client
}

// NewFirebase initialises the Firebase Admin SDK from a credentials file and
// returns auth + messaging clients. Returns nil, nil, nil if the credentials
// file does not exist (local-dev mode: auth rejects tokens, push falls back
// to log-only).
func NewFirebase(ctx context.Context, credentialsFile string) (*FirebaseAuth, *FirebaseMessaging, error) {
	if _, err := os.Stat(credentialsFile); os.IsNotExist(err) {
		slog.Warn("firebase credentials file not found, auth will reject all tokens and push is log-only",
			"path", credentialsFile)
		return nil, nil, nil
	}

	app, err := firebase.NewApp(ctx, nil, option.WithCredentialsFile(credentialsFile))
	if err != nil {
		return nil, nil, fmt.Errorf("firebase init: %w", err)
	}
	authClient, err := app.Auth(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("firebase auth: %w", err)
	}
	msgClient, err := app.Messaging(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("firebase messaging: %w", err)
	}
	return &FirebaseAuth{client: authClient}, &FirebaseMessaging{client: msgClient}, nil
}

// SendToToken delivers one notification to a device token via FCM.
// Returns ErrFCMTokenNotRegistered (wrapped) when the token is stale.
func (f *FirebaseMessaging) SendToToken(ctx context.Context, token, title, body string, data map[string]string) error {
	if f == nil || f.client == nil {
		return fmt.Errorf("firebase messaging is not configured")
	}
	_, err := f.client.Send(ctx, &messaging.Message{
		Token:        token,
		Notification: &messaging.Notification{Title: title, Body: body},
		Data:         data,
	})
	if err != nil {
		if messaging.IsRegistrationTokenNotRegistered(err) {
			return fmt.Errorf("%w: %s", ErrFCMTokenNotRegistered, err)
		}
		return fmt.Errorf("fcm send: %w", err)
	}
	return nil
}

// VerifyIDToken validates a Firebase ID token and returns the user UID.
func (f *FirebaseAuth) VerifyIDToken(ctx context.Context, idToken string) (string, error) {
	if f == nil || f.client == nil {
		return "", fmt.Errorf("firebase auth is not configured")
	}
	token, err := f.client.VerifyIDToken(ctx, idToken)
	if err != nil {
		return "", fmt.Errorf("verify token: %w", err)
	}
	return token.UID, nil
}
