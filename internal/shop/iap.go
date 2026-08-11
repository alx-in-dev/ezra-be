package shop

import (
	"log/slog"
	"os"
)

// ValidateReceipt validates an IAP receipt from Apple or Google.
//
// Real store validation is post-MVP (R3-2). Until then this endpoint must NOT
// be an open faucet — `POST /shop/crystals` takes the amount from the client,
// so a permissive stub means unlimited free crystals for anyone with a
// session. The only accepted receipt is the literal "dev", and only when the
// server explicitly opts in via EZRA_ALLOW_DEV_IAP=1 (local stacks / bot
// swarms); everything else is rejected until real validation lands.
//
// TODO: implement real Apple App Store / Google Play receipt validation
// Apple: POST to https://buy.itunes.apple.com/verifyReceipt
// Google: use androidpublisher API v3
func ValidateReceipt(platform, receipt string) (bool, error) {
	if receipt == "dev" && os.Getenv("EZRA_ALLOW_DEV_IAP") == "1" {
		slog.Info("IAP dev receipt accepted (EZRA_ALLOW_DEV_IAP=1)", "platform", platform)
		return true, nil
	}
	slog.Warn("IAP receipt rejected (real validation not implemented)",
		"platform", platform,
		"receipt_len", len(receipt),
	)
	return false, nil
}
