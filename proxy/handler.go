// HTTP reverse proxy handler
// supports load balancing, discovery of new services to proxy to and authentication
// some of this code comes from the go standard library
// and more specifically from http://golang.org/src/pkg/net/http/httputil/reverseproxy.go
package proxy

import (
	"encoding/json"
	"github.com/gigaroby/authproxy/aerrors"
	"github.com/gigaroby/authproxy/authbroker"
	gorillamux "github.com/gorilla/mux"
	"io/ioutil"
	"net/http"
	"time"
)

type ServiceConf struct {
	Path string `json:"path"`
}

type NotFoundHandler struct{}

func (h *NotFoundHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	err := aerrors.ResponseError{
		Code:    "error.notFound",
		Message: "API endpoint not found",
		Status:  404,
	}

	writeError(rw, err)
}

func NewProxyHandler(b authbroker.AuthenticationBroker, t http.RoundTripper, servicesFile, backendsFile string) http.Handler {
	if t == nil {
		t = http.DefaultTransport
	}

	if b == nil {
		b = &authbroker.YesBroker{}
	}

	services := make(map[string]ServiceConf)
	content, err := ioutil.ReadFile(servicesFile)
	if err != nil {
		logger.Fatal(err.Error())
	}
	err = json.Unmarshal(content, &services)
	if err != nil {
		logger.Fatal(err.Error())
	}

	mux := gorillamux.NewRouter()
	mux.NotFoundHandler = &NotFoundHandler{}

	for k, v := range services {
		d := &JsonDiscoverer{Path: backendsFile, Name: k}
		lb := NewLoadBalancer(d, &RandomRouter{}, time.Duration(1)*time.Second)
		lb.Start()
		sh := NewServiceHandler(k, &v, t, b, lb)
		sh.Register(mux)
	}

	return mux
}

func copyHeader(dst, src http.Header) {
	//TODO[vad]: it doesn't preserve the headers again... it seems that src already has broken case
	for k, vv := range src {
		// NOTE: don't use Add here, it doesn't preserve the case: https://code.google.com/p/go/issues/detail?id=5022
		dst[k] = vv
	}
	for _, k := range []string{"X-DL-Count", "X-DL-datagem-version"} {
		s := dst.Get(k)
		if s != "" {
			dst.Del(k)
			dst[k] = []string{s}
		}
	}
}
