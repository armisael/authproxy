package proxy

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"
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

func TestThreeScaleBrokerPOSTRequests(t *testing.T) {
	recorder := &RecordTransport{}
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

func TestThreeScaleBrokerAuthenticateSupportsLimits(t *testing.T) {
	body :=
		`<?xml version="1.0" encoding="UTF-8"?>
        <status>
            <authorized>true</authorized>
            <plan>Default</plan>
            <usage_reports>
                <usage_report metric="hits" period="day">
                    <period_start>2013-10-01 00:00:00 +0000</period_start>
                    <period_end>2013-10-02 00:00:00 +0000</period_end>
                    <max_value>10000000</max_value>
                    <current_value>2</current_value>
                </usage_report>
              </usage_reports>
        </status>`
	factory := &FactoryTransport{Response: NewResponse(200, body)}
	broker := NewThreeScaleBroker("providerKey", factory)

	data := url.Values{}
	data.Set("$app_id", "MyApp")
	data.Set("$app_key", "MyKey")

	req, _ := http.NewRequest("POST", "http://example.com", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, msg, _ := broker.Authenticate(req)

	if msg["creditsLeft"] != "9999998" {
		t.Error("Expected 9999998 credits left, got", msg["creditsLeft"])
	}

	if msg["creditsReset"] != "2013-10-02 00:00:00 +0000" {
		t.Error("Resets expected to be '2013-10-02 00:00:00 +0000', got", msg["creditsReset"])
	}
}

func TestThreeScaleBrokerAuthenticateWorksWithMonthlyLimits(t *testing.T) {
	body :=
		`<?xml version="1.0" encoding="UTF-8"?>
        <status>
            <authorized>true</authorized>
            <plan>Default</plan>
            <usage_reports>
                <usage_report metric="hits" period="month">
                    <period_start>2013-10-01 00:00:00 +0000</period_start>
                    <period_end>2013-11-01 00:00:00 +0000</period_end>
                    <max_value>100</max_value>
                    <current_value>10</current_value>
                </usage_report>
              </usage_reports>
        </status>`
	factory := &FactoryTransport{Response: NewResponse(200, body)}
	broker := NewThreeScaleBroker("providerKey", factory)

	data := url.Values{}
	data.Set("$app_id", "MyApp")
	data.Set("$app_key", "MyKey")

	req, _ := http.NewRequest("POST", "http://example.com", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, msg, _ := broker.Authenticate(req)

	if msg["creditsLeft"] != "90" {
		t.Error("Expected 90 credits left, got", msg["creditsLeft"])
	}

	if msg["creditsReset"] != "2013-11-01 00:00:00 +0000" {
		t.Error("Resets expected to be '2013-11-01 00:00:00 +0000', got", msg["creditsReset"])
	}
}

func TestThreeScaleBrokerAuthenticateWorksWithBothDailyAndMonthlyLimits(t *testing.T) {
	// ATM we prefer daily over monthly. Probably we should check the lower one (the one with current_value near max_value)
	body :=
		`<?xml version="1.0" encoding="UTF-8"?>
        <status>
            <authorized>true</authorized>
            <plan>Default</plan>
            <usage_reports>
                <usage_report metric="hits" period="month">
                    <period_start>2013-10-01 00:00:00 +0000</period_start>
                    <period_end>2013-11-01 00:00:00 +0000</period_end>
                    <max_value>100</max_value>
                    <current_value>10</current_value>
                </usage_report>
                <usage_report metric="hits" period="day">
                    <period_start>2013-10-01 00:00:00 +0000</period_start>
                    <period_end>2013-10-02 00:00:00 +0000</period_end>
                    <max_value>20</max_value>
                    <current_value>2</current_value>
                </usage_report>
              </usage_reports>
        </status>`
	factory := &FactoryTransport{Response: NewResponse(200, body)}
	broker := NewThreeScaleBroker("providerKey", factory)

	data := url.Values{}
	data.Set("$app_id", "MyApp")
	data.Set("$app_key", "MyKey")

	req, _ := http.NewRequest("POST", "http://example.com", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, msg, _ := broker.Authenticate(req)

	if msg["creditsLeft"] != "18" {
		t.Error("Expected 18 credits left, got", msg["creditsLeft"], ". Perhaps it read monthly limits instead")
	}
}

func TestThreeScaleBrokerReportSetsHeaders(t *testing.T) {
	factory := &FactoryTransport{Response: NewResponse(200, "")}
	broker := NewThreeScaleBroker("providerKey", factory)
	res := NewResponse(200, "")

	msg := map[string]string{
		"appId":        "MyApp",
		"appKey":       "MyKey",
		"creditsLeft":  "20000000",
		"creditsReset": "over the rainbow",
	}

	broker.Report(res, msg)

	for header, expected := range map[string]string{"X-DL-credits": "1", "X-DL-credits-reset": "over the rainbow", "X-DL-credits-left": "19"} {
		if res.Header[header][0] != expected {
			t.Error(header, "HTTP header is missing or wrong. Expected: '",
				expected, "', got ", res.Header[header][0])
		}
	}
}
