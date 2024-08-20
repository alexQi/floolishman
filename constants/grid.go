package constants

type GridStatus string

var (
	GridStatusOut           GridStatus = "OUT"
	GridStatusInside        GridStatus = "INSIDE"
	GridStatusMoreLimit     GridStatus = "LIMIT"
	GridStatusGreaterAvgVol GridStatus = "GREATE_AVG_VOL"
)
