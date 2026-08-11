package shop

import "time"

type Purchase struct {
	ID            string    `json:"id" db:"id"`
	PlayerID      string    `json:"player_id" db:"player_id"`
	ItemType      string    `json:"item_type" db:"item_type"`
	ItemID        string    `json:"item_id" db:"item_id"`
	CrystalsSpent int       `json:"crystals_spent" db:"crystals_spent"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

type Subscription struct {
	PlayerID  string    `json:"player_id" db:"player_id"`
	Type      string    `json:"type" db:"type"`
	Active    bool      `json:"active" db:"active"`
	StartedAt time.Time `json:"started_at" db:"started_at"`
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
	Receipt   string    `json:"-" db:"receipt"`
}

type ShopItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"` // acceleration, expansion, cosmetic
	Cost     int    `json:"cost"`     // crystals
}

var Catalog = []ShopItem{
	// Acceleration
	{ID: "instant_build_1h", Name: "Мгновенное строительство (≤1ч)", Category: "acceleration", Cost: 20},
	{ID: "instant_build_long", Name: "Мгновенное строительство (>1ч)", Category: "acceleration", Cost: 50},
	{ID: "army_heal", Name: "Восстановление армии", Category: "acceleration", Cost: 30},
	{ID: "pet_speedup", Name: "Ускорение питомца", Category: "acceleration", Cost: 15},
	// Expansion
	{ID: "storage_500", Name: "Хранилище +500⚡", Category: "expansion", Cost: 100},
	{ID: "storage_2000", Name: "Хранилище +2000⚡", Category: "expansion", Cost: 350},
	{ID: "pet_slot", Name: "Слот питомца", Category: "expansion", Cost: 500},
	{ID: "army_limit_10", Name: "Армия +10", Category: "expansion", Cost: 200},
	// Cosmetic
	{ID: "beacon_skin", Name: "Скин маяка", Category: "cosmetic", Cost: 150},
	{ID: "pet_skin", Name: "Скин питомца", Category: "cosmetic", Cost: 300},
	{ID: "dome_color", Name: "Цвет купола", Category: "cosmetic", Cost: 80},
}
