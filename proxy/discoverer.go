package proxy

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"sync"
)

type ServiceDiscoverer interface {
	// A service discoverer is the component responsable for
	// finding out about all the instances to proxy requests to (services).
	// It does that by calling Discover() which returns a list of
	// urls to proxy the request to or an error.
	Discover() ([]Service, error)
}

type StaticDiscoverer struct {
	// StaticDiscoverer is the simplest possible implementation of
	// ServiceDiscoverer.
	// It just returns a predefined list of urls.
	// Its Discover method doesn't return an error unless Services is empty.
	Services []Service
	mu       sync.Mutex
}

func (s *StaticDiscoverer) Discover() (services []Service, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Services) < 1 {
		return nil, fmt.Errorf("no services are avaible")
	}
	return s.Services, nil
}

type FileDiscoverer struct {
	// FileDiscverer reads the list of services from a file.
	// Each line of the file should contain only one URL.
	// Returns error either if there was an error opening the file
	// or if no URLs were specified in the file.
	Path string
}

func (f *FileDiscoverer) Discover() (services []Service, err error) {
	file, err := os.Open(f.Path)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		url, err := url.Parse(scanner.Text())
		if err != nil {
			continue
		}
		services = append(services, Service(*url))
	}

	if len(services) == 0 {
		err = fmt.Errorf("no services specified in file [%s]\n", f.Path)
	}
	return
}
