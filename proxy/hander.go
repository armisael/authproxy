// HTTP reverse proxy handler
// supports load balancing, discovery of new services to proxy to and authentication
// some of this code comes from the go standard library
// and more specifically from http://golang.org/src/pkg/net/http/httputil/reverseproxy.go
package proxy

import (
    "net"
    "net/http"
    "net/url"
    "strings"
    "io"
)

type Service url.URL

// ProxyHandler takes an incoming http request
// proxying it to one of the backend services.
type ProxyHandler struct {

    Balancer LoadBalancer

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



func (p *ProxyHandler) requestToProxy(inreq *http.Request, proxyService *Service) *http.Request{
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

    service, _ := p.Balancer.NextService()
    outreq := p.requestToProxy(req, service)

    res, _ := transport.RoundTrip(outreq)
    defer res.Body.Close()
    copyHeader(rw.Header(), res.Header)
    rw.WriteHeader(res.StatusCode)
    io.Copy(rw, res.Body)
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}
