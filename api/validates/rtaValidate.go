package validates

import (
	"encoding/json"
	"errors"
	"floolishman/api/typing"
	"floolishman/utils"
	"floolishman/utils/encrypt"
	"floolishman/utils/validate"
	"github.com/kataras/iris/v12"
)

// 自定义错误消息
func rtaFieldTrans() map[string]string {
	return map[string]string{
		"Channel.required":  "channel must be required,",
		"Device.required":   "device must be required",
		"Platform.required": "platform must be required",
		"Platform.oneof":    "platform was invalid",
	}
}

// RecvRequestValidate 进行验证
func RtaRequestValidate(ctx iris.Context) (typing.RtaRequest, error) {
	rtaRequest := typing.RtaRequest{}
	if err := ctx.ReadJSON(&rtaRequest); err != nil {
		utils.Log.Errorf("read post param json error")
		return rtaRequest, err
	}
	// 判断设备参数
	return RunValidate(rtaRequest, ctx)
}

// RunValidate
func RunValidate(request typing.RtaRequest, ctx iris.Context) (typing.RtaRequest, error) {
	if request.MediaNo == "" {
		request.MediaNo = "0001"
	}
	// 处理未加密的参数
	if request.Device.Imei != "" {
		request.Device.ImeiMd5 = encrypt.Md5(request.Device.Imei)
	}
	if request.Device.Oaid != "" {
		request.Device.OaidMd5 = encrypt.Md5(request.Device.Oaid)
	}
	if request.Device.Idfa != "" {
		request.Device.IdfaMd5 = encrypt.Md5(request.Device.Idfa)
	}
	if request.Device.ImeiMd5 == "" && request.Device.OaidMd5 == "" && request.Device.IdfaMd5 == "" {
		return request, errors.New("device params error: must be has one")
	}
	// 输出debug日志
	requestString, _ := json.Marshal(request)
	utils.Log.Debugf("request-%s-%s-%s params: %s", request.Platform, request.Channel, request.MediaNo, requestString)
	// 执行validator
	return request, validate.Run(request, rtaFieldTrans())
}
