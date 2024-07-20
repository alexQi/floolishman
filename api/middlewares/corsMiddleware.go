package middlewares

import (
	corsMiddleware "github.com/iris-contrib/middleware/cors"
	"github.com/kataras/iris/v12"
	"github.com/spf13/viper"
)

func CorsNew() iris.Handler {
	corsHandler := corsMiddleware.New(corsMiddleware.Options{
		AllowedOrigins:   viper.GetStringSlice("cors.AllowedOrigins"), //允许通过的主机名称
		AllowCredentials: viper.GetBool("cors.AllowCredentials"),
		AllowedHeaders:   viper.GetStringSlice("cors.AllowedHeaders"),
		ExposedHeaders:   viper.GetStringSlice("cors.ExposedHeaders"),
		AllowedMethods:   viper.GetStringSlice("cors.AllowedMethods"),
		Debug:            viper.GetBool("cors.Debug"),
	})
	return corsHandler
}
