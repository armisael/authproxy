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
	creditsHeader      = "X-DL-credits"
	creditsLeftHeader  = "X-DL-credits-left"
	creditsResetHeader = "X-DL-credits-reset"
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
	Report(*http.Request, *http.Response, BrokerMessage) error
}

// YesBroker is to be used for debug only.
type YesBroker struct{}

func (y *YesBroker) Authenticate(req *http.Request) (toProxy bool, err *ResponseError, msg BrokerMessage) {
	toProxy = true
	return
}

func (y *YesBroker) Report(req *http.Request, res *http.Response, msg BrokerMessage) (err error) {
	return
}

// 3scale broker http://3scale.net
type ThreeScaleBroker struct {
	ProviderKey string
	Transport   http.RoundTripper
}

type ThreeXMLStatus struct {
	XMLName    xml.Name
	Data       string `xml:",chardata"` // text-content of the root element
	Authorized bool   `xml:"authorized"`
	Reason     string `xml:"reason"`
	Plan       string `xml:"plan"`
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

	if appKey == "" || appKey == "" {
		err = &ResponseError{Message: "missing parameters $app_id and/or $app_key",
			Status: 401, Code: "api.auth.unauthorized"}
		return
	}
	msg = make(map[string]string)
	msg["appId"] = appId

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

	toProxy = status.Authorized
	err = &ResponseError{Message: status.Reason, Status: 401, Code: "api.auth.unauthorized"}

	return
}

func (brk *ThreeScaleBroker) Report(req *http.Request, res *http.Response, msg BrokerMessage) (err error) {
	appId := msg["appId"]
	credits, creditsErr := strconv.Atoi(res.Header.Get(creditsHeader))

	if creditsErr != nil {
		logger.Info("The response from ", req.URL.String(), " does not contain ", creditsHeader)
		credits = 1
		res.Header[creditsHeader] = []string{strconv.Itoa(credits)}
	}
	hits := credits * 1000000

	values := url.Values{
		"provider_key":                 {brk.ProviderKey},
		"transactions[0][app_id]":      {appId},
		"transactions[0][usage][hits]": {strconv.Itoa(hits)},
	}

	repRes, err := http.PostForm("https://su1.3scale.net/transactions.xml", values)

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
