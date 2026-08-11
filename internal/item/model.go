package item

// Item is a stack of a player-owned inventory item. Resonance signatures use
// Variant to record their spectrum (the collectible set for the D1 endgame);
// fungible items (keys) leave Variant empty.
type Item struct {
	ItemType  string `json:"item_type" db:"item_type"`
	Variant   string `json:"variant" db:"variant"`
	Qty       int    `json:"qty" db:"qty"`
	Name      string `json:"name" db:"-"`      // computed display name
	SpectrumRu string `json:"spectrum_ru,omitempty" db:"-"`
}

// Item types.
const (
	TypeResonanceSignature = "resonance_signature" // dropped by critical-rift shard bosses (D5 → D1)
	TypeHackKey            = "hack_key"             // fungible key (pet finds / future use)
	TypeBlueprintFragment  = "blueprint_fragment"   // rare drop; 3 open a Spire build (megastructure)
	TypeSeasonTitle        = "season_title"          // faction-war season reward (variant = season key)

	// TypePityBlueprint is an INTERNAL counter (not shown in inventory): the
	// player's consecutive-miss streak for the blueprint-fragment rare drop.
	// Hidden from ListByPlayer; see Service.RollBlueprintFragment (audit C2).
	TypePityBlueprint = "pity_blueprint"
)

// ResonanceSpectra is the canonical set of signature spectra a player collects
// from shard bosses. Completing the set drives the D1 regional counter-wave.
// Themed after the spirit "spectrum" from the source films/GD bible.
var ResonanceSpectra = []string{"ember", "tide", "gale", "verdant", "umbra"}

var spectrumRu = map[string]string{
	"ember":   "Уголь",
	"tide":    "Прилив",
	"gale":    "Вихрь",
	"verdant": "Зелень",
	"umbra":   "Тень",
}

// SpectrumRu returns the Russian display name for a spectrum key.
func SpectrumRu(spectrum string) string {
	if ru, ok := spectrumRu[spectrum]; ok {
		return ru
	}
	return spectrum
}

// decorate fills computed display fields.
func (i *Item) decorate() {
	switch i.ItemType {
	case TypeResonanceSignature:
		i.SpectrumRu = SpectrumRu(i.Variant)
		i.Name = "Резонансная сигнатура · " + i.SpectrumRu
	case TypeHackKey:
		i.Name = "Ключ взлома"
	case TypeBlueprintFragment:
		i.Name = "Осколок чертежа"
	case TypeSeasonTitle:
		i.Name = "Титул сезона " + i.Variant
	default:
		i.Name = i.ItemType
	}
}
