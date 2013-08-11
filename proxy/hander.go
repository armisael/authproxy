// HTTP reverse proxy handler
// supports load balancing, discovery of new services to proxy to and authentication
// some of this code comes from the go standard library
// and more specifically from http://golang.org/src/pkg/net/http/httputil/reverseproxy.go
package proxy

import (
    "net"
    "net/http"
    "net/url"
    "time"
    "strings"
)

// maximum number of times that 
// service discovery and proxy request will be attempted
// before an error is reported.
const MAX_ATTEMPTS = 3
// time to wait between attempts
const ATTEMPT_DELAY = 100 * time.Millisecond

// ProxyHandler takes an incoming http request
// proxying it to one of the backend services.
type ProxyHandler struct {
    // Discoverer is responsable for returning the
    // list of all the backend services.
    Discoverer ServiceDiscoverer
    // The router decides where the request will be routed.
    Router RequestRouter
    // The broker is responsable for the authentication
    // and authorization of the request.
    Broker AuthenticationBroker

    // The transport used to perform proxy requests.
	// If nil, http.DefaultTransport is used.
    Transport http.RoundTripper
}

// Hop-by-hop headers. These are removed when sent to the backend.
// http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html
var hopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te", // canonicalized version of "TE"
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
}

func attempt(maxRetray int, retrayDelay time.Duration, toAttempt func() error) error{
    i := 0
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

func (p *ProxyHandler) requestForProxy(inreq *http.Request, proxyService *url.URL) *http.Request{
    outreq := new(http.Request)
    *outreq = *inreq

	outreq.Proto = "HTTP/1.1"
	outreq.ProtoMajor = 1
	outreq.ProtoMinor = 1
	outreq.Close = false
    // force http for now
    // this proxy is going to work inside a LAN anyway
    outreq.URL.Scheme = "http"
    outreq.URL.Host = proxyService.Host
    outreq.URL.Path = proxyService.Path
    outreq.URL.RawQuery = proxyService.RawQuery

	// Remove hop-by-hop headers to the backend.  Especially
	// important is "Connection" because we want a persistent
	// connection, regardless of what the client sent to us.  This
	// is modifying the same underlying map from req (shallow
	// copied above) so we only copy it if necessary.
	copiedHeaders := false
	for _, h := range hopHeaders {
		if outreq.Header.Get(h) != "" {
			if !copiedHeaders {
				outreq.Header = make(http.Header)
				copyHeader(outreq.Header, inreq.Header)
				copiedHeaders = true
			}
			outreq.Header.Del(h)
		}
	}

    if clientIP, _, err := net.SplitHostPort(inreq.RemoteAddr); err == nil {
		// If we aren't the first proxy retain prior
		// X-Forwarded-For information as a comma+space
		// separated list and fold multiple headers into one.
		if prior, ok := outreq.Header["X-Forwarded-For"]; ok {
			clientIP = strings.Join(prior, ", ") + ", " + clientIP
		}
		outreq.Header.Set("X-Forwarded-For", clientIP)
	}

    return outreq
}

func (p *ProxyHandler) getProxyServices() ([]url.URL, error){
    var (
        err error
        allServices []url.URL
    )

    // try at most 3 times waiting 100 milliseconds between each try
    err = attempt(3, 100*time.Millisecond, func() error{
        allServices, err = p.Discoverer.Discover()
        return err
    })

    return allServices, err
}

func (p *ProxyHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
    transport := p.Transport

    if transport == nil {
        transport = http.DefaultTransport
    }

    authorized, autherr := p.Broker.Authenticate(req)

    if !authorized {
        rw.Header().Set("Content-Type", autherr.ContentType)
        rw.WriteHeader(autherr.Code)
        rw.Write([]byte(autherr.Message))
        return
    }

    //TODO: finish up here

    outreq := p.requestForProxy(req, service)
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}
