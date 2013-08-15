package proxy

import (
    "sync"
    "time"
    "log"
)

type LoadBalancer struct {
    // Discoverer is responsable for returning the
    // list of all the backend services.
    // the call to Discover should be thread-safe (goroutine safe?)
    Discoverer *ServiceDiscoverer
    // The router decides where the request will be routed.
    Router *RequestRouter
    // How often do we update services list
    FetchDelay time.Duration
    // Services should be requested on this channel
    Services chan Service

    cachedServices []Service

    mu sync.Mutex
    started bool
    quit chan chan bool
}

func NewLoadBalancer(d *ServiceDiscoverer, r *RequestRouter) *LoadBalancer {
    ldb := &LoadBalancer{
        Discoverer: d,
        Router: r,
        started: false,
        quit: make(chan chan bool),
    }

    return ldb
}

// There should be only one goroutine calling Start
// at any given time
func (l *LoadBalancer) Start() error {
    if !l.started {
    }
    return nil
}


func attempt(maxRetray int, retrayDelay time.Duration, toAttempt func() error) error{
    var i int = 0
    for {
        err := toAttempt()
        if err != nil {
            if i < maxRetray {
                time.Sleep(retrayDelay)
                i++
                continue
            }
            return err
        } else {
            return nil
        }
    }
}
