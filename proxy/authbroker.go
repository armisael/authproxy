package proxy

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
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

// A authentication broker is the component that authenticates
// incoming requests to decide if they should be routed or not.
type AuthenticationBroker interface {
	Authenticate(*http.Request) (bool, *ResponseError)
	Report(req *http.Request, res *http.Response) error
}

// YesBroker is to be used for debug only.
type YesBroker struct{}

func (y *YesBroker) Authenticate(req *http.Request) (toProxy bool, err *ResponseError) {
	return true, nil
}

func (y *YesBroker) Report(req *http.Request, res *http.Response) error {
	return nil
}

// 3scale broker http://3scale.net

type ThreeXMLStatus struct {
	XMLName    xml.Name
	Data       string `xml:",chardata"` // text-content of the root element
	Authorized bool   `xml:"authorized"`
	Reason     string `xml:"reason"`
	Plan       string `xml:"plan"`
}

type ThreeScaleBroker struct {
	ProviderKey string
}

func (brk *ThreeScaleBroker) Authenticate(req *http.Request) (toProxy bool, err *ResponseError) {
	client := &http.Client{}

	reqValues := req.URL.Query()

	values := url.Values{}
	values.Set("provider_key", brk.ProviderKey)
	values.Set("app_id", reqValues.Get("$app_id"))
	values.Set("app_key", reqValues.Get("$app_key"))

	authReq, _ := http.NewRequest("GET", "https://su1.3scale.net/transactions/authorize.xml", nil)
	authReq.URL.RawQuery = values.Encode()

	authRes, err_ := client.Do(authReq)
	if err_ != nil {
		//TODO[vad]: report 3scale's down
		log.Fatalf("Error connecting to 3scale: %s\n", err_)
	}
	defer authRes.Body.Close()

	buf := new(bytes.Buffer)
	buf.ReadFrom(authRes.Body)

	status := ThreeXMLStatus{}
	xml.Unmarshal(buf.Bytes(), &status)

	if status.XMLName.Local == "error" {
		return false, &ResponseError{Message: status.Data, Status: 401, Code: "api.auth.unauthorized"}
	}

	return status.Authorized, &ResponseError{
		Message: status.Reason, Status: 401, Code: "api.auth.unauthorized"}
}

func (brk *ThreeScaleBroker) Report(req *http.Request, res *http.Response) (err error) {
	reqValues := req.URL.Query()
	app_id := reqValues.Get("$app_id")

	values := url.Values{
		"provider_key":                 {brk.ProviderKey},
		"transactions[0][app_id]":      {app_id},
		"transactions[0][usage][hits]": {"10"},
	}

	repRes, err := http.PostForm("http://su1.3scale.net/transactions.xml", values)

	if err != nil {
		return
	}

	if repRes.StatusCode == 202 {
		log.Println("3scale report ok!")
		return nil
	}

	return errors.New(fmt.Sprintf("Error reporting to 3scale API: status code %d", repRes.StatusCode))
}
