package proxy

import (
    "fmt"
    "net/url"
)

// A service discoverer is the component responsable for
// finding out about all the instances to proxy requests to (services).
// It does that by calling Discover() which returns a list of
// urls to proxy the request to or an error.
type ServiceDiscoverer interface {
    Discover() ([]url.URL, error)
}

// StaticDiscoverer is the simplest possible implementation of
// ServiceDiscoverer.
// It just returns a predefined list of urls.
// Its Discover method doesn't return an error unless Services is empty.
type StaticDiscoverer struct{
    Services []url.URL
}

func (s *StaticDiscoverer) Discover() (services []url.URL, err error){
    if len(s.Services) < 1 {
        return nil, fmt.Errorf("no services are avaible")
    }
    return s.Services, nil
}
