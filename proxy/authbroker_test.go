package proxy

import (
	"bytes"
	. "github.com/smartystreets/goconvey/convey"
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

func noProviderBroker(transport http.RoundTripper) *ThreeScaleBroker {
	return NewThreeScaleBroker("providerKey", nil, transport)
}

func TestThreeScaleBrokerPOSTRequests(t *testing.T) {
	recorder := &RecordTransport{}
	broker := noProviderBroker(recorder)

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
	broker := noProviderBroker(factory)

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
	broker := noProviderBroker(factory)

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
	broker := noProviderBroker(factory)

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
	broker := noProviderBroker(factory)
	res := NewResponse(200, "")
	res.Header.Set("X-DL-units", "0.1")

	msg := map[string]string{
		"appId":        "MyApp",
		"appKey":       "MyKey",
		"creditsLeft":  "20000000",
		"creditsReset": "over the rainbow",
	}

	broker.Report(res, msg)

	for header, expected := range map[string]string{"X-Dl-Units": "0.1", "X-DL-units-reset": "over the rainbow", "X-DL-units-left": "19.9"} {
		headerValues := res.Header[header]
		if len(headerValues) != 1 {
			t.Error("Wrong number of HTTP header ", header, ". Expected: 1, got:", len(headerValues))
		} else if headerValues[0] != expected {
			t.Error(header, "HTTP header is missing or wrong. Expected: '",
				expected, "', got ", headerValues[0])
		}
	}
}

func TestThreeScaleBrokerReportWorks(t *testing.T) {
	transport := &RecordTransport{}
	broker := noProviderBroker(transport)

	Convey("Given a backend response", t, func() {
		Convey("When it contains units as a floating point number", func() {
			res := NewResponse(200, "")
			res.Header.Set("X-DL-units", "0.02")
			broker.Report(res, BrokerMessage{})

			Convey("It reports them to 3scale", func() {
				bBody, _ := ioutil.ReadAll(transport.LastRequest.Body)
				body := string(bBody)

				So(body, ShouldContainSubstring, "usage%5D%5Bhits%5D=20000")
			})
		})

		Convey("When it contains units as an integer", func() {
			res := NewResponse(200, "")
			res.Header.Set("X-DL-units", "5")
			broker.Report(res, BrokerMessage{})

			Convey("It reports them to 3scale", func() {
				bBody, _ := ioutil.ReadAll(transport.LastRequest.Body)
				body := string(bBody)

				So(body, ShouldContainSubstring, "usage%5D%5Bhits%5D=5000000")
			})
		})
	})
}
