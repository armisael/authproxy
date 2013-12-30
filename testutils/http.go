package testutils

import (
	"bytes"
	"io/ioutil"
	"net/http"
)

func NewResponse(statusCode int, bodyString string) *http.Response {
	body := ioutil.NopCloser(bytes.NewBuffer([]byte(bodyString)))
	return &http.Response{StatusCode: statusCode, Body: body, Header: make(http.Header)}
}

// a http.RoundTripper that records *http.Requests
type RecordTransport struct {
	Requests    []*http.Request
	LastRequest *http.Request
}

func (rec *RecordTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec.LastRequest = req
	rec.Requests = append(rec.Requests, req)
	return NewResponse(500, ""), nil
}

// a http.RoundTripper that returns predefined *http.Response
type FactoryTransport struct {
	Response *http.Response
}

func (t *FactoryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.Response, nil
}
