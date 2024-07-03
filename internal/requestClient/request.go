package requestClient

import (
	"crypto/tls"
	"net/http"
	"runtime"
	"time"
)

func New() *http.Client {

	tr := &http.Transport{
		MaxIdleConns:      runtime.NumCPU() * 250,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		IdleConnTimeout:   200 * time.Microsecond,
		DisableKeepAlives: true,
	}
	return &http.Client{
		Transport: tr,
		Timeout:   0 * time.Microsecond,
	}
}
