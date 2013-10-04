package authserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gigaroby/authproxy/admin"
	"github.com/gigaroby/authproxy/proxy"
	"github.com/vad/go-bunyan/bunyan"
	"net/http"
)

const (
	requestMaxSize = 1 << 20
)

var (
	logger = bunyan.NewLogger("authproxy.admin")
)

type Handle struct {
	mux http.Handler
}

func status(rw http.ResponseWriter, req *http.Request) {
	rw.WriteHeader(200)
	rw.Write([]byte("ok"))
}

type ClosingReader struct {
	bytes.Reader
}

func (rnc ClosingReader) Close() error {
	return nil
}

type responseJson struct {
	Error   bool   `json:"error"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
	Status  int    `json:"status"`
}

func NewHandle(broker *proxy.ThreeScaleBroker, proxyHandler *proxy.ProxyHandler, adminPath string) *Handle {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", status)
	// TODO[vad] this should only be enabled with 3scale broker
	creditsHandler := &admin.CreditsHandle{Broker: broker}
	mux.Handle(fmt.Sprintf("/%s/credits", adminPath), creditsHandler)
	mux.Handle("/", proxyHandler)

	return &Handle{mux: mux}
}

func (h *Handle) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()
	req.Body = http.MaxBytesReader(rw, req.Body, requestMaxSize)

	// limit the request Body and buffer it
	var buffer bytes.Buffer
	_, readErr := buffer.ReadFrom(req.Body)
	defer req.Body.Close()

	if readErr != nil {
		logger.Info(readErr.Error())
		rw.WriteHeader(400)
		res, _ := json.Marshal(&responseJson{Status: 400, Message: "Request too large", Code: "error.requestTooLarge"})
		rw.Write(res)
		return
	}

	body := ClosingReader{*bytes.NewReader(buffer.Bytes())}
	req.Body = &body

	h.mux.ServeHTTP(rw, req)
}
