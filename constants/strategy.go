package constants

import (
	"floolishman/model"
	"floolishman/strategies"
)

var ConstStraties = map[string]model.Strategy{
	"Test":              &strategies.Test{},
	"Range15m":          &strategies.Range15m{},
	"MomentumVolume15m": &strategies.MomentumVolume15m{},
	"Rsi1h":             &strategies.Rsi1h{},
	"Resonance15m":      &strategies.Resonance15m{},
	"Rsi15m":            &strategies.Rsi15m{},
	"Vibrate15m":        &strategies.Vibrate15m{},
	"Kc15m":             &strategies.Kc15m{},
	"Macd15m":           &strategies.Macd15m{},
	"Scoop":             &strategies.Scoop{},
	"Scooper":           &strategies.Scooper{},
	"ScooperWeight":     &strategies.ScooperWeight{},
	"Radicalization":    &strategies.Radicalization{},
	"Grid1h":            &strategies.Grid1h{},
}
