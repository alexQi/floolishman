package api

import (
	"floolishman/api/controllers"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/core/router"
)

func HealthRoutes(app router.Party) {
	c := controllers.HealthController{}

	app.Get("/live", func(ctx iris.Context) {
		c.Live(ctx)
	})
}
