package types

import (
	"time"

	"floolisher/model"
)

type MetricStyle string

type IndicatorMetric struct {
	Name   string
	Color  string
	Style  MetricStyle // default: line
	Values model.Series[float64]
}

type ChartIndicator struct {
	Time      []time.Time
	Metrics   []IndicatorMetric
	Overlay   bool
	GroupName string
	Warmup    int
}
