package api

import (
	"floolishman/api/process"
)

func init() {
	go process.CacheSetting()
}
