package api

import (
	"floolishman/api/controllers"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/core/router"
)

func PairRoutes(app router.Party) {
	c := controllers.PairController{}
	// 定义路由handler 加载速率限制中间件
	app.Get("/switchStatus", func(ctx iris.Context) {
		_ = c.SwitchStatus(ctx)
	})
}
