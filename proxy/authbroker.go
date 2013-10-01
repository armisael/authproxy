package proxy

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const (
	creditsHeader            = "X-DL-credits"
	creditsLeftHeader        = "X-DL-credits-left"
	creditsResetHeader       = "X-DL-credits-reset"
	threeScaleHitsMultiplier = int(1e6)
)

type ResponseError struct {
	Status      int
	Message     string
	ContentType string
	Code        string
}

func (r ResponseError) Error() string {
	return r.Message
}

// a cache to pass parameters between Authenticate and Report
type BrokerMessage map[string]string

// A authentication broker is the component that authenticates
// incoming requests to decide if they should be routed or not.
type AuthenticationBroker interface {
	Authenticate(*http.Request) (bool, *ResponseError, BrokerMessage)
	Report(*http.Response, BrokerMessage) error
}

// YesBroker is to be used for debug only.
type YesBroker struct{}

func (y *YesBroker) Authenticate(req *http.Request) (toProxy bool, err *ResponseError, msg BrokerMessage) {
	toProxy = true
	return
}

func (y *YesBroker) Report(res *http.Response, msg BrokerMessage) (err error) {
	return
}

// 3scale broker http://3scale.net
type ThreeScaleBroker struct {
	ProviderKey string
	Transport   http.RoundTripper
}

type ThreeXMLUsageReport struct {
	MaxValue     int    `xml:"max_value"`
	CurrentValue int    `xml:"current_value"`
	PeriodStart  string `xml:"period_start"`
	PeriodEnd    string `xml:"period_end"`
	Metric       string `xml:"metric,attr"`
	Period       string `xml:"period,attr"`
}

type ThreeXMLStatus struct {
	XMLName      xml.Name
	Data         string                `xml:",chardata"` // text-content of the root element
	Authorized   bool                  `xml:"authorized"`
	Reason       string                `xml:"reason"`
	Plan         string                `xml:"plan"`
	UsageReports []ThreeXMLUsageReport `xml:"usage_reports>usage_report"`
}

func NewThreeScaleBroker(provKey string, transport http.RoundTripper) *ThreeScaleBroker {
	if transport == nil {
		transport = &http.Transport{
		// TODO[vad]: use the dial timeout from main
		// Dial: dialTimeout,
		}
	}
	return &ThreeScaleBroker{ProviderKey: provKey, Transport: transport}
}

func parseRequestForApp(req *http.Request) (appId, appKey string) {
	switch req.Method {
	case "GET":
		{
			reqValues := req.URL.Query()
			appId = reqValues.Get("$app_id")
			appKey = reqValues.Get("$app_key")
		}
	default: // POST or PUT
		{
			appId = req.PostFormValue("$app_id")
			appKey = req.PostFormValue("$app_key")
		}
	}

	return
}

func (brk *ThreeScaleBroker) Authenticate(req *http.Request) (toProxy bool, err *ResponseError, msg BrokerMessage) {
	client := &http.Client{Transport: brk.Transport}

	appId, appKey := parseRequestForApp(req)

	values := url.Values{}
	values.Set("provider_key", brk.ProviderKey)
	values.Set("app_id", appId)
	values.Set("app_key", appKey)
	// TODO[vad]: we should send Hits=1 too. ATM we go down to -1 requests left (and we show it to the user!)

	if appKey == "" || appKey == "" {
		err = &ResponseError{Message: "missing parameters $app_id and/or $app_key",
			Status: 401, Code: "api.auth.unauthorized"}
		return
	}
	msg = map[string]string{
		"appId": appId,
	}

	authReq, _ := http.NewRequest("GET", "https://su1.3scale.net/transactions/authorize.xml", nil)
	authReq.URL.RawQuery = values.Encode()

	authRes, err_ := client.Do(authReq)
	if err_ != nil {
		//TODO[vad]: report 3scale's down
		logger.Err("Error connecting to 3scale: ", err.Error())
		return
	}
	if authRes.Body == nil {
		logger.Err("Broken response from 3scale (empty body)")
		return
	}
	defer authRes.Body.Close()

	buf := new(bytes.Buffer)
	buf.ReadFrom(authRes.Body)

	status := ThreeXMLStatus{}
	xml.Unmarshal(buf.Bytes(), &status)

	if status.XMLName.Local == "error" {
		err = &ResponseError{Message: status.Data, Status: 401, Code: "api.auth.unauthorized"}
		return
	}

	// create a new slice with only daily limits
	var dailyUsageReports []ThreeXMLUsageReport
	for _, report := range status.UsageReports {
		if report.Period == "day" {
			dailyUsageReports = append(dailyUsageReports, report)
		}
	}

	if len(dailyUsageReports) != 1 {
		logger.Warning("Missing/too much usage reports for app_id ", appId, ". Expected 1, got ", len(dailyUsageReports))
	} else {
		msg["creditsLeft"] = strconv.Itoa(dailyUsageReports[0].MaxValue - dailyUsageReports[0].CurrentValue)
		msg["creditsReset"] = dailyUsageReports[0].PeriodEnd
	}

	toProxy = status.Authorized
	err = &ResponseError{Message: status.Reason, Status: 401, Code: "api.auth.unauthorized"}

	return
}

func (brk *ThreeScaleBroker) Report(res *http.Response, msg BrokerMessage) (err error) {
	client := &http.Client{Transport: brk.Transport}

	appId := msg["appId"]
	credits, creditsErr := strconv.Atoi(res.Header.Get(creditsHeader))

	if creditsErr != nil {
		if res.Request != nil {
			logger.Info("The response from ", res.Request.URL.String(), " does not contain ", creditsHeader)
		}
		credits = 1
		res.Header[creditsHeader] = []string{strconv.Itoa(credits)}
	}
	hits := credits * threeScaleHitsMultiplier
	if msg["creditsLeft"] != "" {
		creditsLeft, _ := strconv.Atoi(msg["creditsLeft"])
		res.Header[creditsLeftHeader] = []string{strconv.Itoa((creditsLeft - hits) / threeScaleHitsMultiplier)}
	}
	if msg["creditsReset"] != "" {
		res.Header[creditsResetHeader] = []string{msg["creditsReset"]}
	}

	values := url.Values{
		"provider_key":                 {brk.ProviderKey},
		"transactions[0][app_id]":      {appId},
		"transactions[0][usage][hits]": {strconv.Itoa(hits)},
	}

	repRes, err := client.PostForm("https://su1.3scale.net/transactions.xml", values)

	// if there was an error in the HTTP request, return it
	if err != nil {
		return
	}

	// if 202, it's ok
	if repRes.StatusCode == 202 {
		logger.Debug("3scale report ok!")
		return nil
	}

	// an unmanaged status code from 3scale, report it
	return fmt.Errorf("Error reporting to 3scale API for app %s: status code %d", appId,
		repRes.StatusCode)
}
