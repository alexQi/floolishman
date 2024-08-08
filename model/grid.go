package model

import (
	"sort"
	"time"
)

// 定义网格
type PositionGrid struct {
	BasePrice     float64
	CreatedAt     time.Time
	CountGrid     int64
	BoundaryUpper float64
	BoundaryLower float64
	GridItems     []PositionGridItem
}

// 网格间隔
type PositionGridItem struct {
	Lock         bool
	Side         SideType
	PositionSide PositionSideType
	Price        float64
}

func (pg *PositionGrid) SortGridItemsByPrice(asc bool) {
	sort.Slice(pg.GridItems, func(i, j int) bool {
		if asc {
			return pg.GridItems[i].Price < pg.GridItems[j].Price
		} else {
			return pg.GridItems[i].Price > pg.GridItems[j].Price
		}
	})
}
