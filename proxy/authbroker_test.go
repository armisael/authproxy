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

func TestThreeScaleBrokerAuthenticate(t *testing.T) {
	transport := &RecordTransport{}
	broker := noProviderBroker(transport)

	data := url.Values{}
	data.Set("$app_id", "MyApp")
	data.Set("$app_key", "MyKey")

	Convey("Given a proxy using the 3scale broker", t, func() {
		// TODO[vad]: add a GET request

		Convey("When a POST request to /datatxt/nex/v1 arrives", func() {
			req, _ := http.NewRequest("POST", "http://example.com/datatxt/nex/v1", strings.NewReader(data.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			broker.Authenticate(req)
			recorded := transport.LastRequest

			Convey("Then there should be a GET request to 3scale", func() {
				So(recorded.Method, ShouldEqual, "GET")
			})

			query := recorded.URL.Query()
			Convey("Then the request should have the right app_id and app_key", func() {
				So(query.Get("app_id"), ShouldEqual, "MyApp")
				So(query.Get("app_key"), ShouldEqual, "MyKey")
			})

			Convey("Then the request should send the 'method name'", func() {
				So(query.Get("usage[datatxt/nex/v1]"), ShouldEqual, "1")
			})
		})
	})
}

func TestThreeScaleBrokerAuthenticateLimits(t *testing.T) {
	bodyDaily :=
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
	bodyMonthly :=
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
	bodyDailyMonthly :=
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

	data := url.Values{}
	data.Set("$app_id", "MyApp")
	data.Set("$app_key", "MyKey")

	req, _ := http.NewRequest("POST", "http://example.com", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	Convey("Given a user with limits", t, func() {
		Convey("When the user has daily limits", func() {
			factory := &FactoryTransport{Response: NewResponse(200, bodyDaily)}
			broker := noProviderBroker(factory)
			_, msg, _ := broker.Authenticate(req)

			Convey("Then the Authenticate() should read credits left correctly", func() {
				So(msg["creditsLeft"], ShouldEqual, "9999998")
			})
			Convey("Then the Authenticate() should read next credit reset correctly", func() {
				So(msg["creditsReset"], ShouldEqual, "2013-10-02 00:00:00 +0000")
			})
		})
		Convey("When the user has monthly limits", func() {
			factory := &FactoryTransport{Response: NewResponse(200, bodyMonthly)}
			broker := noProviderBroker(factory)
			_, msg, _ := broker.Authenticate(req)

			Convey("Then the Authenticate() should read credits left correctly", func() {
				So(msg["creditsLeft"], ShouldEqual, "90")
			})
			Convey("Then the Authenticate() should read next credit reset correctly", func() {
				So(msg["creditsReset"], ShouldEqual, "2013-11-01 00:00:00 +0000")
			})
		})
		Convey("When the user has both daily and monthly limits", func() {
			factory := &FactoryTransport{Response: NewResponse(200, bodyDailyMonthly)}
			broker := noProviderBroker(factory)
			_, msg, _ := broker.Authenticate(req)

			Convey("Then the Authenticate() should read credits left correctly", func() {
				So(msg["creditsLeft"], ShouldEqual, "18")
			})
			Convey("Then the Authenticate() should read next credit reset correctly", func() {
				So(msg["creditsReset"], ShouldEqual, "2013-10-02 00:00:00 +0000")
			})
		})
	})
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

	for header, expected := range map[string]string{"X-DL-units": "0.1", "X-DL-units-reset": "over the rainbow", "X-DL-units-left": "19.9"} {
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
			wait, _ := broker.Report(res, BrokerMessage{"metricName": "datatxt/nex/v1"})
			<-wait

			Convey("It reports them to 3scale", func() {
				bBody, _ := ioutil.ReadAll(transport.LastRequest.Body)
				body := string(bBody)
				sub := url.QueryEscape("[usage][datatxt/nex/v1]") + "=20000"

				So(body, ShouldContainSubstring, sub)
			})
		})

		Convey("When it contains units as an integer", func() {
			res := NewResponse(200, "")
			res.Header.Set("X-DL-units", "5")
			wait, _ := broker.Report(res, BrokerMessage{"metricName": "datatxt/nex/v1"})
			<-wait

			Convey("It reports them to 3scale", func() {
				bBody, _ := ioutil.ReadAll(transport.LastRequest.Body)
				body := string(bBody)
				sub := url.QueryEscape("[usage][datatxt/nex/v1]") + "=5000000"

				So(body, ShouldContainSubstring, sub)
			})
		})
	})
}
