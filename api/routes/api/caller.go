package api

import (
	"floolishman/api/controllers"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/core/router"
)

func CallerRoutes(app router.Party) {
	c := controllers.CallerController{}
	// 定义路由handler 加载速率限制中间件
	app.Get("/switchStatus", func(ctx iris.Context) {
		_ = c.SwitchStatus(ctx)
	})
}
