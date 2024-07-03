package httputil

import (
	"floolishman/utils"
	"io"
	"net/url"
	"reflect"
)

func BuildParams(params interface{}) url.Values {
	args := url.Values{}

	t := reflect.TypeOf(params)
	v := reflect.ValueOf(params)
	for k := 0; k < t.NumField(); k++ {
		if v.FieldByName(t.Field(k).Name).String() == "" {
			continue
		}
		args.Set(t.Field(k).Tag.Get("json"), v.FieldByName(t.Field(k).Name).String())
	}
	return args
}

func BodyCloser(Body io.ReadCloser) {
	err := Body.Close()
	if err != nil {
		utils.Log.Errorf("http response error:%s", err.Error())
		return
	}
}
