package proxy

import (
	"log"
	"sync"
	"time"
)

type LoadBalancer struct {
	// Discoverer is responsable for returning the
	// list of all the backend services.
	Discoverer ServiceDiscoverer
	// The router decides where the request will be routed.
	Router RequestRouter
	// How often do we update services list
	FetchInterval time.Duration
	// Services should be requested on this channel
	Services chan Service

	mu             sync.Mutex
	cachedServices []Service
	started        bool
	quit           chan chan bool
}

func NewLoadBalancer(d ServiceDiscoverer, r RequestRouter, fi time.Duration) *LoadBalancer {
	ldb := &LoadBalancer{
		Discoverer:    d,
		Router:        r,
		FetchInterval: fi,

		Services: make(chan Service),
		started:  false,
		quit:     make(chan chan bool),
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
	defer l.mu.Unlock()
	err := l.fetchUnsafe()
	// should try more than once and be able to configure this
	// err := attempt(3, 100*time.Millisecond, l.fetchUnsafe)
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
	tick := time.NewTicker(l.FetchInterval)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			l.fetch()
		case l.Services <- l.nextService():
			break
		case quitchan := <-l.quit:
			quitchan <- true
			return
		}
	}
}
