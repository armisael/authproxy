package proxy

import (
    "math/rand"
)

// The request router is the component that decides
// where the next request is going to be routed.
// Route takes as an argument every avaiable
// service and returns the service to proxy the request to.
type RequestRouter interface {
    Route ([]Service) Service
}

// The RandomRouter is the simplest implementation of
// RequestRouter. Its Route method randomly selects
// a service and returns it.
// The call to Route in a RandomRouter will never return
// an error.
type RandomRouter struct {}
func (r *RandomRouter) Route (urls []Service) Service {
    rnd := rand.Int() % len(urls)
    return urls[rnd]
}
