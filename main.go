package main

import (
	"flag"
	"fmt"
	"github.com/gigaroby/authproxy/proxy"
	"log"
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
)

func dialTimeout(network, addr string) (net.Conn, error) {
	return net.DialTimeout(network, addr, timeout)
}

func main() {
	flag.StringVar(&providerKey, "3scale-provider-key", "", "3scale provider key")
	flag.Parse()

	if providerKey == "" {
		log.Fatalln("Missing parameter -3scale-provider-key")
	}

	loadb := proxy.NewLoadBalancer(
		&proxy.FileDiscoverer{Path: *serviceFile},
		&proxy.RandomRouter{},
		1*time.Second,
	)

	err := loadb.Start()
	if err != nil {
		log.Fatalf("can't fetch initial server list\n")
	}

	broker := &proxy.ThreeScaleBroker{ProviderKey: providerKey}

	transport := http.Transport{
		Dial: dialTimeout,
	}

	handler := proxy.NewProxyHandler(loadb, broker, &transport, *subpath)

	server := &http.Server{
		Addr:    PROXY_PORT,
		Handler: handler,
	}

	fmt.Printf("proxy listening on %s\n", server.Addr)
	fmt.Printf("proxying requests to: %s", loadb.GetCache())
	fmt.Println(server.ListenAndServe())
	loadb.WaitStop()
}
