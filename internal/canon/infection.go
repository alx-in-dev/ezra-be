package canon

const (
	InfectionBandSafe      = "safe"
	InfectionBandAlert     = "alert"
	InfectionBandDangerous = "dangerous"
	InfectionBandCritical  = "critical"
	InfectionBandRupture   = "rupture"
)

func InfectionBand(infection float64) string {
	switch {
	case infection <= 20:
		return InfectionBandSafe
	case infection <= 40:
		return InfectionBandAlert
	case infection <= 60:
		return InfectionBandDangerous
	case infection <= 80:
		return InfectionBandCritical
	default:
		return InfectionBandRupture
	}
}
