package proxy

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type RecordTripper struct {
	req *http.Request
}

func (rec *RecordTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rec.req = req
	body := ioutil.NopCloser(bytes.NewBuffer([]byte("")))
	return &http.Response{Status: "500", Body: body}, nil
}

func TestThreeScaleBrokerPOSTRequests(t *testing.T) {
	recorder := new(RecordTripper)
	broker := NewThreeeScaleBroker("providerKey", recorder)

	data := url.Values{}
	data.Set("$app_id", "MyApp")
	data.Set("$app_key", "MyKey")

	req, _ := http.NewRequest("POST", "http://example.com", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	broker.Authenticate(req)

	recorded := recorder.req

	if recorded.Method != "GET" {
		t.Error("Expected GET to 3scale, got", req.Method)
	}

	query := recorded.URL.Query()
	if query.Get("app_id") != "MyApp" || query.Get("app_key") != "MyKey" {
		t.Error("Missing app_id or app_key in 3scale API call")
	}
}
