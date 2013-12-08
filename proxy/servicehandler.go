package proxy

import (
	"encoding/json"
	gorillamux "github.com/gorilla/mux"
	"io"
	"math"
	"net"
	"net/http"
	"strings"
	"time"
)

type ServiceHandler struct {
	Path      string
	Transport http.RoundTripper
	Broker    AuthenticationBroker
	Balancer  LoadBalancer
}

func NewServiceHandler(name string, conf *ServiceConf, t http.RoundTripper, b AuthenticationBroker, lb *LoadBalancer) *ServiceHandler {
	return &ServiceHandler{
		Path:      (*conf).Path,
		Transport: t,
		Broker:    b,
		Balancer:  *lb,
	}
}

func (h *ServiceHandler) Register(mux *gorillamux.Router) {
	if h.Path[len(h.Path)-1] != '/' {
		(*mux).Handle(h.Path+"/", h)
	}
	(*mux).Handle(h.Path, h)
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

func (p *ServiceHandler) requestToProxy(inreq *http.Request, proxyService Service) *http.Request {
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

func writeError(rw http.ResponseWriter, err ResponseError) {
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

func (p *ServiceHandler) doProxyRequest(req *http.Request) (res *http.Response, d time.Duration, outErr error) {
	proxyService := <-p.Balancer.Services
	// proxyService := Service{}

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
			Message: "can't connect to the backend server",
			Status:  http.StatusBadGateway,
			Code:    "error.badGateway",
		}
	}

	return
}

func (h *ServiceHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	var (
		err     error
		reqData = map[string]interface{}{
			"remote_address": req.RemoteAddr,
			"url":            req.URL.String(),
		}
	)
	logger.Debugm("request initiated", reqData)

	if len(req.URL.RawQuery) > 7001 {
		writeError(rw, ResponseError{Message: "The requested URI is too long for a GET, please use POSTs",
			Status: 414, Code: "error.requestURITooLong"})
		return
	}

	// if !strings.HasPrefix(req.URL.Path, p.path) {
	//  writeError(rw, ResponseError{Message: "Not found",
	//      Status: 404, Code: "error.notFound"})
	//  return
	// } else { // strip that prefix
	//  req.URL.Path = req.URL.Path[len(p.path):]
	// }
	req.URL.Path = ""

	authorized, msg, err := h.Broker.Authenticate(req)

	if !authorized {
		writeError(rw, *err.(*ResponseError))
		return
	}

	var res *http.Response
	var duration time.Duration

	err = attempt(3, 50*time.Millisecond, func() error {
		if seeker, ok := req.Body.(io.Seeker); ok {
			seeker.Seek(0, 0)
		}
		res, duration, err = h.doProxyRequest(req)
		return err
	})

	url := req.URL.String()
	shortURL := url[:int(math.Min(200, float64(len(url))))]
	reqData["url"] = shortURL
	reqData["type"] = "request"
	reqData["duration"] = duration
	reqData["status"] = res.StatusCode

	if err != nil {
		resError := err.(ResponseError)
		reqData["status"] = resError.Status
		logger.Errorm("error proxing request", reqData)
		writeError(rw, resError)
		return
	}
	defer res.Body.Close()

	logger.Infom("request completed successfully", reqData)

	if _, reportErr := h.Broker.Report(res, msg); reportErr != nil {
		logger.Error("Report call failed, but the show must go on!")
	}

	copyHeader(rw.Header(), res.Header)
	rw.WriteHeader(res.StatusCode)
	io.Copy(rw, res.Body)
}
