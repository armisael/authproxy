package proxy

import (
	"net/http"
)

type ResponseError struct {
	Message     string
	ContentType string
	Code        int
}

func (r ResponseError) Error() string {
	return r.Message
}

// A authentication broker is the component that authenticates
// incoming requests to decide if they should be routed or not.
type AuthenticationBroker interface {
	Authenticate(*http.Request) (bool, *ResponseError)
}

// YesBroker is to be used for debug only.
type YesBroker struct{}

func (y *YesBroker) Authenticate(req *http.Request) (toProxy bool, err *ResponseError) {
	return true, nil
}
