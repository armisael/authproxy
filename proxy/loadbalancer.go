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
    Discoverer ServiceDiscoverer
    // The router decides where the request will be routed.
    Router RequestRouter
    // How often do we update services list
    FetchDelay time.Duration
    // Services should be requested on this channel
    Services chan Service

    mu sync.Mutex
    cachedServices []Service
    started bool
    quit chan chan bool
}

func NewLoadBalancer(d ServiceDiscoverer, r RequestRouter) LoadBalancer {
    ldb := LoadBalancer{
        Discoverer: d,
        Router: r,
        started: false,
        quit: make(chan chan bool),
    }

    return ldb
}

func (l *LoadBalancer) Start() error {
    l.mu.Lock()
    defer l.mu.Unlock()
    if l.started {
        return nil
    }
    err := l.fetchUnsafe()
    if err != nil {
        return err
    } else {
        go l.loop()
        l.started = true
        return nil
    }

}

func (l *LoadBalancer) WaitStop() {
    <-l.Stop()
}

func (l *LoadBalancer) Stop() chan bool {
    quitChan := make(chan bool)
    l.quit <- quitChan
    return quitChan
}

func (l *LoadBalancer) fetchUnsafe() error {
    newServices, err := l.Discoverer.Discover()
    if err != nil {
        return err
    }
    l.cachedServices = newServices
    return nil
}

func (l *LoadBalancer) fetch() {
    l.mu.Lock()
    err := l.fetchUnsafe()
    defer l.mu.Unlock()
    //err := attempt(3, 100*time.Millisecond, l.fetchUnsafe)

    if err != nil {
        log.Printf("unable to fetch updated service list\n")
    }
}

func (l *LoadBalancer) nextService() Service {
    l.mu.Lock()
    defer l.mu.Unlock()
    return l.Router.Route(l.cachedServices)
}

func (l *LoadBalancer) loop() {
    tick := time.NewTicker(1 * time.Second)
    defer tick.Stop()
    for {
        select {
        case <-tick.C:
            l.fetch()
        case l.Services <-l.nextService():
            continue
        case quitchan := <-l.quit:
            defer func(){ quitchan <-true }()
            return
        }
    }
}

func attempt(maxRetray int, retrayDelay time.Duration, attemptFunc func() error) error{
    var i int = 0
    for {
        err := attemptFunc()
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
