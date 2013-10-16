package authserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gigaroby/authproxy/admin"
	"github.com/gigaroby/authproxy/proxy"
	"github.com/vad/go-bunyan/bunyan"
	"io"
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

func NewHandle(broker proxy.AuthenticationBroker, proxyHandler http.Handler, adminPath string) *Handle {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", status)

	if tBroker, ok := broker.(*proxy.ThreeScaleBroker); ok {
		creditsHandler := &admin.CreditsHandle{Broker: tBroker}
		mux.Handle(fmt.Sprintf("/%s/credits", adminPath), creditsHandler)
	}
	mux.Handle("/", proxyHandler)

	return &Handle{mux: mux}
}

func limitAndBufferBody(rw http.ResponseWriter, body io.ReadCloser, requestMaxSize int64) (rc io.ReadCloser, err error) {
	var buffer bytes.Buffer

	// limit the request Body and buffer it
	_, err = buffer.ReadFrom(http.MaxBytesReader(rw, body, requestMaxSize))

	if err != nil {
		return
	}

	// we can't use NopCloser here, because we need to Seek after.
	// ClosingReader (with its anonymous field) allows to do it
	rc = &ClosingReader{*bytes.NewReader(buffer.Bytes())}
	return
}

func (h *Handle) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()

	body, err := limitAndBufferBody(rw, req.Body, requestMaxSize)
	req.Body = body

	if err != nil {
		logger.Info(err.Error())
		rw.WriteHeader(400)
		res, _ := json.Marshal(&responseJson{Status: 400, Message: "Request too large", Code: "error.requestTooLarge"})
		rw.Write(res)
		return
	}

	h.mux.ServeHTTP(rw, req)
}
