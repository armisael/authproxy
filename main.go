package main

import(
    "fmt"
    "log"
    "math/rand"
    "net/url"
    "net/http"
    "net/http/httputil"
)

const (
    PROXY_PORT = ":8080"
)

type ServiceDiscoverer interface {
    Discover() ([]url.URL, error)
}

type RequestRouter interface {
    Route ([]url.URL) (url.URL, error)
}

// Need to understand specifications for this
type AuthenticationBroker interface {}

type ProxyBackend struct {
    Router RequestRouter
    Discoverer ServiceDiscoverer
    //Broker AuthenticationBroker
}

// Retrieve the next url to proxy the request to
// according to the specified Discoverer and Router
func (p *ProxyBackend) nextURL() (url.URL, error) {
    avaibleServices, err := p.Discoverer.Discover()
    if err != nil {
        log.Println(err)
        return url.URL{}, err
    }

    nextURL, err := p.Router.Route(avaibleServices)
    if err != nil {
        log.Println(err)
        return url.URL{}, err
    }

    return nextURL, nil
}

// The job of the director function is to modify the request
// into a new one to be sent to the proxied service.
func (p *ProxyBackend) DirectorFunction(req *http.Request){
    //don't try to recover errors, for now
    serviceURL, _ := p.nextURL()
    req.URL.Scheme = serviceURL.Scheme
    req.URL.Host = serviceURL.Host
    req.URL.Path = serviceURL.Path
    req.URL.RawQuery = serviceURL.RawQuery
}

type RandomRouter struct {}
func (r *RandomRouter) Route (urls []url.URL) (url.URL, error){
    rnd := rand.Int() % len(urls)
    return urls[rnd], nil
}

type StaticDiscoverer struct{
    Services []url.URL
}
func (s *StaticDiscoverer) Discover() ([]url.URL, error){
    return s.Services, nil
}

func main(){
    var u1, u2 *url.URL
    u1, _ = url.Parse("http://yasse.eu")
    u2, _ = url.Parse("http://dandelion.eu")
    backend := &ProxyBackend{
        Router: &RandomRouter{},
        Discoverer: &StaticDiscoverer{Services: []url.URL{*u1, *u2}},
    }
    server := &http.Server{
        Addr: PROXY_PORT,
        Handler: &httputil.ReverseProxy{
            Director: backend.DirectorFunction,
        },
    }

    fmt.Printf("proxy listening on %s\n", server.Addr)
    fmt.Println(server.ListenAndServe())
}
