package proxy

import (
    "net/http"
)

// Just a proof of concept.
type ErrorResponse struct {
    Message string
    ContentType string
    Code int
}

// A authentication broker is the component that authenticates
// incoming requests to decide if they should be routed or not.
type AuthenticationBroker interface {
    Authenticate (*http.Request) (bool, *ErrorResponse)
}

// YesBroker is to be used for debug only.
type YesBroker struct {}
func (y *YesBroker) Authenticate(req *http.Request) (toProxy bool, err *ErrorResponse){
    return true, nil
}

