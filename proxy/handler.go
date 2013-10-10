// HTTP reverse proxy handler
// supports load balancing, discovery of new services to proxy to and authentication
// some of this code comes from the go standard library
// and more specifically from http://golang.org/src/pkg/net/http/httputil/reverseproxy.go
package proxy

import (
	"encoding/json"
	"io"
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

	// copy inreq to outreq
	url := *inreq.URL
	outreq.URL = &url

	// force http for now
	// this proxy is going to work inside a LAN anyway
	outreq.URL.Scheme = "http"
	outreq.URL.Host = proxyService.Host
	outreq.URL.Path = proxyService.Path + inreq.URL.Path

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

func (p *ProxyHandler) doProxyRequest(req *http.Request) (res *http.Response, d time.Duration, outErr error) {
	proxyService := <-p.Balancer.Services

	outReq := p.requestToProxy(req, proxyService)

	// p.Transport is always set in New function
	start := time.Now()
	res, err := p.Transport.RoundTrip(outReq)
	d = time.Now().Sub(start)
	if err != nil {
		netError, ok := err.(net.Error)
		if ok && netError.Timeout() {
			logger.Info("The Backend timed out: ", err.Error())
		}

		outErr = ResponseError{
			Message: err.Error(),
			Status:  http.StatusBadGateway,
			Code:    "error.badGateway",
		}
	}

	return
}

func (p *ProxyHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	logger.Debug("got request from ", req.RemoteAddr, " on ", req.URL)
	var err error

	if !strings.HasPrefix(req.URL.Path, p.path) {
		p.writeError(rw, ResponseError{Message: "Not found",
			Status: 404, Code: "error.notFound"})
		return
	} else { // strip that prefix
		req.URL.Path = req.URL.Path[len(p.path):]
	}

	authorized, msg, err := p.Broker.Authenticate(req)

	if !authorized {
		p.writeError(rw, *err.(*ResponseError))
		return
	}

	var res *http.Response
	var duration time.Duration

	err = attempt(3, 50*time.Millisecond, func() error {
		if seeker, ok := req.Body.(io.Seeker); ok {
			seeker.Seek(0, 0)
		}
		res, duration, err = p.doProxyRequest(req)
		return err
	})

	if err != nil {
		logger.Err("error proxying request for ", req.URL, " to backend. error was: ", err)
		p.writeError(rw, err.(ResponseError))
		return
	}
	defer res.Body.Close()

	logger.Info("Remote service called successfully, it last ", duration)

	if reportErr := p.Broker.Report(res, msg); reportErr != nil {
		logger.Err("Report call failed, but the show must go on!")
	}

	copyHeader(rw.Header(), res.Header)
	rw.WriteHeader(res.StatusCode)
	io.Copy(rw, res.Body)
}

func copyHeader(dst, src http.Header) {
	//TODO[vad]: it doesn't preserve the headers again... it seems that src already has broken case
	for k, vv := range src {
		// NOTE: don't use Add here, it doesn't preserve the case: https://code.google.com/p/go/issues/detail?id=5022
		dst[k] = vv
	}
}
