// HTTP reverse proxy handler
// supports load balancing, discovery of new services to proxy to and authentication
// some of this code comes from the go standard library
// and more specifically from http://golang.org/src/pkg/net/http/httputil/reverseproxy.go
package proxy

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// ProxyHandler takes an incoming http request
// proxying it to one of the backend services.
type ProxyHandler struct {
	Balancer *LoadBalancer

	// The broker is responsable for the authentication
	// and authorization of the request.
	Broker AuthenticationBroker

	// The transport used to perform proxy requests.
	// If nil, http.DefaultTransport is used.
	Transport http.RoundTripper

	// route requests on this path only. 404 the others
	path string
}

func NewProxyHandler(l *LoadBalancer, b AuthenticationBroker, t http.RoundTripper, path string) *ProxyHandler {
	if t == nil {
		t = http.DefaultTransport
	}

	if b == nil {
		b = &YesBroker{}
	}

	return &ProxyHandler{
		Balancer:  l,
		Broker:    b,
		Transport: t,
		path:      path,
	}
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

func (p *ProxyHandler) requestToProxy(inreq *http.Request, proxyService Service) *http.Request {
	var outreq *http.Request = new(http.Request)
	*outreq = *inreq

	outreq.Proto = "HTTP/1.1"
	outreq.ProtoMajor = 1
	outreq.ProtoMinor = 1
	outreq.Close = false
	// force http for now
	// this proxy is going to work inside a LAN anyway
	outreq.URL.Scheme = "http"
	outreq.URL.Host = proxyService.Host
	outreq.URL.Path = proxyService.Path + outreq.URL.Path

	// we need to pass query params. In the future we can merge
	//   inreq.RawQuery and proxyService.RawQuery
	// outreq.URL.RawQuery = proxyService.RawQuery

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

type JSONError struct {
	Error   bool                   `json:"error"`
	Status  int                    `json:"status"`
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data"`
}

func (p *ProxyHandler) writeError(rw http.ResponseWriter, err ResponseError) {
	if err.ContentType != "" {
		rw.Header().Set("Content-Type", err.ContentType)
	} else {
		rw.Header().Set("Content-Type", "application/json")
	}
	rw.WriteHeader(err.Status)
	marshalled, _ := json.Marshal(JSONError{
		Status:  err.Status,
		Message: err.Message,
		Error:   true,
		Data:    make(map[string]interface{}),
		Code:    err.Code,
	})
	rw.Write(marshalled)
}

func (p *ProxyHandler) doProxyRequest(req *http.Request) (res *http.Response, outErr error) {
	proxyService := <-p.Balancer.Services
	outReq := p.requestToProxy(req, proxyService)

	// p.Transport is always set in New function
	res, err := p.Transport.RoundTrip(outReq)
	if err != nil {
		netError, ok := err.(net.Error)
		if ok && netError.Timeout() {
			outErr = ResponseError{
				Message: err.Error(),
				Status:  http.StatusGatewayTimeout,
				Code:    "api.net.timeout",
			}
		} else {
			outErr = ResponseError{
				Message: err.Error(),
				Status:  http.StatusBadGateway,
				Code:    "api.net.badGateway",
			}
		}
	}

	return
}

func (p *ProxyHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	log.Printf("got request from %s\n", req.RemoteAddr)
	var err error

	if req.URL.Path == "/status" {
		rw.WriteHeader(200)
		rw.Write([]byte("ok"))
		return
	}

	if !strings.HasPrefix(req.URL.Path, p.path) {
		p.writeError(rw, ResponseError{Message: "Not found",
			Status: 404, Code: "api.notFound"})
		return
	} else { // strip that prefix
		req.URL.Path = req.URL.Path[len(p.path):]
	}

	authorized, err := p.Broker.Authenticate(req)

	if !authorized {
		p.writeError(rw, *err.(*ResponseError))
		return
	}

	var res *http.Response

	err = attempt(3, 100*time.Millisecond, func() error {
		res, err = p.doProxyRequest(req)
		return err
	})

	if err != nil {
		log.Println("error proxying request for", req.URL, "to backend. error was:", err)
		p.writeError(rw, err.(ResponseError))
		return
	}

	reportErr := p.Broker.Report(req, res)

	if reportErr != nil {
		log.Println("Report call failed, but the show must go on!")
	}

	defer res.Body.Close()
	copyHeader(rw.Header(), res.Header)
	rw.WriteHeader(res.StatusCode)
	io.Copy(rw, res.Body)
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		// don't use Add here, it doesn't preserve the case: https://code.google.com/p/go/issues/detail?id=5022
		dst[k] = vv
	}
}
