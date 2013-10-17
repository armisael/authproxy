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
	creditsHeader            = "X-DL-units"
	creditsLeftHeader        = "X-DL-units-left"
	creditsResetHeader       = "X-DL-units-reset"
	ThreeScaleHitsMultiplier = 1e6
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
	Authenticate(*http.Request) (bool, BrokerMessage, *ResponseError)
	Report(*http.Response, BrokerMessage) error
}

// YesBroker is to be used for debug only.
type YesBroker struct{}

func (y *YesBroker) Authenticate(req *http.Request) (toProxy bool, msg BrokerMessage, err *ResponseError) {
	toProxy = true
	return
}

func (y *YesBroker) Report(res *http.Response, msg BrokerMessage) (err error) {
	return
}

// 3scale broker http://3scale.net
type ThreeScaleBroker struct {
	ProviderKey             string
	ProviderKeyAlternatives map[string]string
	client                  *http.Client
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

func NewThreeScaleBroker(provKey string, provKeyAlts map[string]string, transport http.RoundTripper) *ThreeScaleBroker {
	if transport == nil {
		transport = &http.Transport{
			// TODO[vad]: use the dial timeout from main
			// Dial: dialTimeout,
			MaxIdleConnsPerHost: 128,
		}
	}

	return &ThreeScaleBroker{ProviderKey: provKey, ProviderKeyAlternatives: provKeyAlts, client: &http.Client{Transport: transport}}
}

func parseRequestForApp(req *http.Request) (appId, appKey, providerLabel string) {
	switch req.Method {
	case "GET":
		{
			reqValues := req.URL.Query()
			appId = reqValues.Get("$app_id")
			appKey = reqValues.Get("$app_key")
			providerLabel = reqValues.Get("$provider")
		}
	default: // POST or PUT
		{
			appId = req.PostFormValue("$app_id")
			appKey = req.PostFormValue("$app_key")
			providerLabel = req.PostFormValue("$provider")
		}
	}

	return
}

func (brk *ThreeScaleBroker) getProviderKey(label string) (providerKey string) {
	providerKey = brk.ProviderKeyAlternatives[label]
	if providerKey == "" {
		providerKey = brk.ProviderKey
	}
	return
}

func (brk *ThreeScaleBroker) DoAuthenticate(appId, appKey, providerLabel string) (status ThreeXMLStatus, msg map[string]string, err *ResponseError) {
	values := url.Values{}

	providerKey := brk.getProviderKey(providerLabel)

	values.Set("app_id", appId)
	values.Set("app_key", appKey)
	values.Set("provider_key", providerKey)
	// TODO[vad]: we should send Hits=1 too. ATM we go down to -1 requests left (and we show it to the user!)

	msg = map[string]string{
		"appId":       appId,
		"providerKey": providerKey,
	}

	authReq, _ := http.NewRequest("GET", "https://su1.3scale.net/transactions/authorize.xml", nil)
	authReq.URL.RawQuery = values.Encode()

	authRes, err_ := brk.client.Do(authReq)
	if err_ != nil {
		//TODO[vad]: report 3scale's down
		logger.Err("Error connecting to 3scale: ", err_.Error())
		err = &ResponseError{Message: "Internal server error", Status: 500, Code: "error.internalServerError"}
		return
	}
	defer authRes.Body.Close()

	// unmarshal the 3scale response
	buf := new(bytes.Buffer)
	buf.ReadFrom(authRes.Body)
	xml.Unmarshal(buf.Bytes(), &status)

	if status.XMLName.Local == "error" {
		err = &ResponseError{Message: status.Data, Status: 401, Code: "error.authenticationError"}
		return
	}

	usageReportsByPeriod := make(map[string][]*ThreeXMLUsageReport)
	for _, report := range status.UsageReports {
		usageReportsByPeriod[report.Period] = append(usageReportsByPeriod[report.Period], &report)
	}

	// find the report we want to show to the user and put it in "report"
	var report *ThreeXMLUsageReport
loop:
	for _, period := range []string{"day", "month"} {
		usageReports := usageReportsByPeriod[period]
		switch len(usageReports) {
		case 0: // no reports for this period, go on
			continue
		case 1:
			{
				report = usageReports[0]
				break loop
			}
		default: // to many reports, why? we need to handle this!
			logger.Warning("Too many usage reports for app_id ", appId, " in period", period, ". Expected 1, got ", len(usageReports))
		}
	}

	if report == nil {
		logger.Warning("Missing usage reports for app_id ", appId)
	} else {
		msg["creditsLeft"] = strconv.Itoa(report.MaxValue - report.CurrentValue)
		msg["creditsReset"] = report.PeriodEnd
	}

	return
}

func (brk *ThreeScaleBroker) Authenticate(req *http.Request) (toProxy bool, msg BrokerMessage, err *ResponseError) {
	appId, appKey, providerLabel := parseRequestForApp(req)

	if appKey == "" || appId == "" {
		err = &ResponseError{Message: "missing parameters $app_id and/or $app_key",
			Status: 401, Code: "error.missingParameter"}
		return
	}

	status, msg, err := brk.DoAuthenticate(appId, appKey, providerLabel)

	if err != nil {
		return
	}

	toProxy = status.Authorized
	err = &ResponseError{Message: status.Reason, Status: 401, Code: "error.authenticationError"}

	return
}

func round(f float64) int {
	return int(f + 0.5)
}

func (brk *ThreeScaleBroker) Report(res *http.Response, msg BrokerMessage) (err error) {
	appId := msg["appId"]
	credits, creditsErr := strconv.ParseFloat(res.Header.Get(creditsHeader), 64)

	if creditsErr != nil {
		if res.Request != nil {
			logger.Info("The response from ", res.Request.URL.String(), " does not contain ", creditsHeader)
		}
		credits = 1.0
		res.Header[creditsHeader] = []string{"1"}
	}
	hits := round(credits * ThreeScaleHitsMultiplier)

	// this should be set by the Authenticate
	if msg["creditsLeft"] != "" {
		creditsLeft, _ := strconv.ParseFloat(msg["creditsLeft"], 64)
		creditsLeftAfter := (creditsLeft - float64(hits)) / float64(ThreeScaleHitsMultiplier)
		res.Header[creditsLeftHeader] = []string{strconv.FormatFloat(creditsLeftAfter, 'f', -1, 64)}
	}
	if msg["creditsReset"] != "" {
		res.Header[creditsResetHeader] = []string{msg["creditsReset"]}
	}

	values := url.Values{
		"provider_key":                 {msg["providerKey"]},
		"transactions[0][app_id]":      {appId},
		"transactions[0][usage][hits]": {strconv.Itoa(hits)},
	}

	repRes, err := brk.client.PostForm("https://su1.3scale.net/transactions.xml", values)

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
