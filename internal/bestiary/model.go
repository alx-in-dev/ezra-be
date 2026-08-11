package bestiary

// Category groups bestiary entries on the client.
type Category string

const (
	CatSpirit   Category = "spirit"   // enemy types seen in battle (persisted discovery)
	CatRift     Category = "rift"     // rift tiers fought (persisted discovery)
	CatTower    Category = "tower"    // beacon types owned (derived from towers)
	CatSpectrum Category = "spectrum" // resonance spectra collected (derived from items)
)

// Entry is one collectible. Key is "<category>:<id>" and is what the discovery
// table / derived queries match against.
type Entry struct {
	Key         string
	Category    Category
	Name        string
	Description string
}

// Catalog is the full bestiary (MVP). Spirit/rift entries are discovered by
// fighting them; tower/spectrum entries are derived from what the player owns.
var Catalog = []Entry{
	{"spirit:small", CatSpirit, "Малый дух", "Рой мелких сущностей — энергоудар их сметает"},
	{"spirit:humanoid", CatSpirit, "Гуманоид", "Тяжёлый дух — оборона гасит его удары"},
	{"spirit:shard", CatSpirit, "Осколок", "Босс критического разлома — элитная броня"},

	{"rift:minor", CatRift, "Малый разлом", "Слабый прорыв заражения (infection 75+)"},
	{"rift:medium", CatRift, "Средний разлом", "Окрепший прорыв (infection 82+)"},
	{"rift:critical", CatRift, "Критический разлом", "Прорыв с осколком-боссом (infection 90+)"},

	{"tower:standard", CatTower, "Стандартный маяк", "Базовый узел сети"},
	{"tower:amplified", CatTower, "Усиленный маяк", "Расширенный энергодиск"},
	{"tower:relay", CatTower, "Ретранслятор", "Тянет сеть дальше"},
	{"tower:symbiont", CatTower, "Симбионт-маяк", "Узел стороны Эзра"},

	{"spectrum:ember", CatSpectrum, "Спектр: Уголь", "Резонансная сигнатура"},
	{"spectrum:tide", CatSpectrum, "Спектр: Прилив", "Резонансная сигнатура"},
	{"spectrum:gale", CatSpectrum, "Спектр: Вихрь", "Резонансная сигнатура"},
	{"spectrum:verdant", CatSpectrum, "Спектр: Зелень", "Резонансная сигнатура"},
	{"spectrum:umbra", CatSpectrum, "Спектр: Тень", "Резонансная сигнатура"},
}

// CompletionRewardCrystals is granted once when every entry is discovered.
const CompletionRewardCrystals = 50

// completionKey is the sentinel discovery row marking the reward as paid.
const completionKey = "bestiary:complete"

// EntryStatus is one entry's client-facing state.
type EntryStatus struct {
	Key         string `json:"key"`
	Category    string `json:"category"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Discovered  bool   `json:"discovered"`
}
