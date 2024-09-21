package constants

import (
	"floolishman/model"
	"floolishman/strategies"
)

var ConstStraties = map[string]model.Strategy{
	"Test15m":           &strategies.Test15m{},
	"Range15m":          &strategies.Range15m{},
	"Momentum15m":       &strategies.Momentum15m{},
	"MomentumVolume15m": &strategies.MomentumVolume15m{},
	"Rsi1h":             &strategies.Rsi1h{},
	"Emacross15m":       &strategies.Emacross15m{},
	"Resonance15m":      &strategies.Resonance15m{},
	"Emacross1h":        &strategies.Emacross1h{},
	"Rsi15m":            &strategies.Rsi15m{},
	"Vibrate15m":        &strategies.Vibrate15m{},
	"Kc15m":             &strategies.Kc15m{},
	"Macd15m":           &strategies.Macd15m{},
	"Scoop":             &strategies.Scoop{},
	"Scooper":           &strategies.Scooper{},
	"Radicalization":    &strategies.Radicalization{},
	"Grid1h":            &strategies.Grid1h{},
}
