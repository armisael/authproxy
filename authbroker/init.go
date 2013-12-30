package authbroker

import (
	"github.com/gigaroby/authproxy/aerrors"
	log "github.com/gigaroby/gopherlog"
	"net/http"
)

const (
	creditsHeader      = "X-DL-units"
	creditsLeftHeader  = "X-DL-units-left"
	creditsResetHeader = "X-DL-units-reset"
)

var (
	logger = log.GetLogger("authproxy.authbroker")
)

// a cache to pass parameters between Authenticate and Report
type BrokerMessage map[string]string

// A authentication broker is the component that authenticates
// incoming requests to decide if they should be routed or not.
type AuthenticationBroker interface {
	Authenticate(*http.Request) (bool, BrokerMessage, *aerrors.ResponseError)
	Report(*http.Response, BrokerMessage) (chan bool, error)
}

// YesBroker is to be used for debug only.
type YesBroker struct{}

func (y *YesBroker) Authenticate(req *http.Request) (toProxy bool, msg BrokerMessage, err *aerrors.ResponseError) {
	toProxy = true
	return
}

func (y *YesBroker) Report(res *http.Response, msg BrokerMessage) (wait chan bool, err error) {
	return
}
