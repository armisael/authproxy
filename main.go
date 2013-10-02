package main

import (
	"flag"
	"github.com/gigaroby/authproxy/proxy"
	"github.com/vad/go-bunyan/bunyan"
	"net"
	"net/http"
	"time"
)

const PROXY_PORT = ":8080"

var (
	serviceFile = flag.String("service-file", "/etc/httproxy/services.conf", "file to load services from")
	subpath     = flag.String("subpath", "/", "allow only requests to this path (and children)")
	timeout     = time.Duration(2) * time.Second // this should be configurable for every service
	providerKey string
	logger      = bunyan.NewLogger("authproxy.main")
)

func dialTimeout(network, addr string) (net.Conn, error) {
	return net.DialTimeout(network, addr, timeout)
}

func status(rw http.ResponseWriter, req *http.Request) {
	rw.WriteHeader(200)
	rw.Write([]byte("ok"))
}

func main() {
	flag.StringVar(&providerKey, "3scale-provider-key", "", "3scale provider key")
	flag.Parse()

	if providerKey == "" {
		logger.Fatal("Missing parameter --3scale-provider-key")
	}

	loadb := proxy.NewLoadBalancer(
		&proxy.FileDiscoverer{Path: *serviceFile},
		&proxy.RandomRouter{},
		1*time.Second,
	)

	err := loadb.Start()
	if err != nil {
		logger.Fatal("can't fetch initial server list")
	}

	broker := proxy.NewThreeScaleBroker(providerKey, nil)

	transport := &http.Transport{
		Dial: dialTimeout,
	}

	proxyHandler := proxy.NewProxyHandler(loadb, broker, transport, *subpath)

	mux := http.NewServeMux()
	mux.HandleFunc("/status", status)
	mux.Handle("/", proxyHandler)

	server := &http.Server{
		Addr:    PROXY_PORT,
		Handler: mux,
	}

	logger.Info("proxy listening on ", server.Addr, ". Proxying requests to: ", loadb.GetCache())
	logger.Info(server.ListenAndServe())
	loadb.WaitStop()
}
