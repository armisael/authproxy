package main

import (
	"fmt"
	"github.com/gigaroby/httproxy/proxy"
	"log"
	"net/http"
	"net/url"
	"time"
)

const PROXY_PORT = ":8080"

func main() {
	var u1, u2 *url.URL
	u1, _ = url.Parse("http://yasse.eu")
	u2, _ = url.Parse("http://dandelion.eu")

	loadb := proxy.NewLoadBalancer(
		&proxy.StaticDiscoverer{Services: []proxy.Service{proxy.Service(*u1), proxy.Service(*u2)}},
		&proxy.RandomRouter{},
		1*time.Second,
	)

	err := loadb.Start()
	if err != nil {
		log.Fatalf("can't fetch initial server list\n")
	}
	handler := &proxy.ProxyHandler{
		Balancer: loadb,
		Broker:   &proxy.YesBroker{},
	}
	server := &http.Server{
		Addr:    PROXY_PORT,
		Handler: handler,
	}

	fmt.Printf("proxy listening on %s\n", server.Addr)
	//fmt.Println(server.ListenAndServe())
	loadb.WaitStop()
	panic("dumping goroutine stack")
}
