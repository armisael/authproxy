package main

import(
    "fmt"
    "net/url"
    "net/http"
    "github.com/gigaroby/httproxy/proxy"
)

const PROXY_PORT = ":8080"

func main(){
    var u1, u2 *url.URL
    u1, _ = url.Parse("http://yasse.eu")
    u2, _ = url.Parse("http://dandelion.eu")
    handler := &proxy.ProxyHandler{
        Router: &proxy.RandomRouter{},
        Discoverer: &proxy.StaticDiscoverer{Services: []url.URL{*u1, *u2}},
        Broker: &proxy.YesBroker{},
    }
    server := &http.Server{
        Addr: PROXY_PORT,
        Handler: handler,
    }

    fmt.Printf("proxy listening on %s\n", server.Addr)
    fmt.Println(server.ListenAndServe())
}
