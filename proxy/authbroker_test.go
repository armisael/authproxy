package proxy

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type RecordTransport struct {
	Requests    []*http.Request
	LastRequest *http.Request
}

func (rec *RecordTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec.LastRequest = req
	rec.Requests = append(rec.Requests, req)
	body := ioutil.NopCloser(bytes.NewBuffer([]byte("")))
	return &http.Response{Status: "500", Body: body}, nil
}

func TestThreeScaleBrokerPOSTRequests(t *testing.T) {
	recorder := new(RecordTransport)
	broker := NewThreeScaleBroker("providerKey", recorder)

	data := url.Values{}
	data.Set("$app_id", "MyApp")
	data.Set("$app_key", "MyKey")

	req, _ := http.NewRequest("POST", "http://example.com", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	broker.Authenticate(req)

	recorded := recorder.LastRequest

	if recorded.Method != "GET" {
		t.Error("Expected GET to 3scale, got", req.Method)
	}

	query := recorded.URL.Query()
	if query.Get("app_id") != "MyApp" || query.Get("app_key") != "MyKey" {
		t.Error("Missing app_id or app_key in 3scale API call")
	}
}
