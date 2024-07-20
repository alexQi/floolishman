package api

import (
	"github.com/kataras/iris/v12/core/router"
	"github.com/kataras/iris/v12/middleware/pprof"
)

func PprofRoutes(app router.Party) {
	// 记载主路由
	app.Get("/debug/pprof", pprof.New())
	// 加载子路由
	app.Get("/debug/pprof/{action:path}", pprof.New())
}
