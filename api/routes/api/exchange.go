package api

import (
	"floolishman/api/controllers"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/core/router"
)

func ExchangeRoutes(app router.Party) {
	c := controllers.ExchangeController{}
	// 定义路由handler 加载速率限制中间件
	app.Get("/getOrder", func(ctx iris.Context) {
		_ = c.GetOrder(ctx)
	})
}
