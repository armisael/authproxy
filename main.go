package main

import (
	"flag"
	"fmt"
	"github.com/gigaroby/authproxy/proxy"
	"log"
	"net/http"
	"time"
)

const PROXY_PORT = ":8080"

var (
	serviceFile = flag.String("service-file", "/etc/httproxy/services.conf", "file to load services from")
)

func main() {
	flag.Parse()

	loadb := proxy.NewLoadBalancer(
		&proxy.FileDiscoverer{Path: *serviceFile},
		&proxy.RandomRouter{},
		1*time.Second,
	)

	err := loadb.Start()
	if err != nil {
		log.Fatalf("can't fetch initial server list\n")
	}

	handler := proxy.NewProxyHandler(loadb, nil, nil)

	server := &http.Server{
		Addr:    PROXY_PORT,
		Handler: handler,
	}

	fmt.Printf("proxy listening on %s\n", server.Addr)
	fmt.Printf("proxying requests to: %s", loadb.GetCache())
	fmt.Println(server.ListenAndServe())
	loadb.WaitStop()
}
