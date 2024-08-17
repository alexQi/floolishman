package controllers

import (
	"floolishman/types"
	"github.com/kataras/iris/v12"
	"strings"
)

type PairController struct {
	BaseController
}

// Check 目标用户检测接口
func (c *PairController) SwitchStatus(ctx iris.Context) error {
	data := map[string]interface{}{
		"code":    "0",
		"message": "success",
	}
	var pair string
	if pair = ctx.URLParamTrim("pair"); len(pair) == 0 {
		data["code"] = "10401"
		data["message"] = "please set pair name"
		return ctx.JSON(data)
	}
	types.PairStatusChan <- strings.ToUpper(pair)
	// 返回响应
	return ctx.JSON(data)
}
