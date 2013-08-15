package main

import(
    //"fmt"
    "time"
    "net/url"
    //"net/http"
    "github.com/gigaroby/httproxy/proxy"
)

//const PROXY_PORT = ":8080"

func main(){
    var u1, u2 *url.URL
    u1, _ = url.Parse("http://yasse.eu")
    u2, _ = url.Parse("http://dandelion.eu")
    loadb := proxy.LoadBalancer {
        Router: &proxy.RandomRouter{},
        Discoverer: &proxy.StaticDiscoverer{Services: []proxy.Service{proxy.Service(*u1), proxy.Service(*u2)}},
        FetchDelay: 5*time.Second,
    }
    loadb.Start()
    //handler := &proxy.ProxyHandler{
    //    Balancer: loadb,
    //    Broker: &proxy.YesBroker{},
    //}
    //server := &http.Server{
    //    Addr: PROXY_PORT,
    //    Handler: handler,
    //}

    //fmt.Printf("proxy listening on %s\n", server.Addr)
    //fmt.Println(server.ListenAndServe())
    <-loadb.Stop()
    panic("dumping goroutine stack")
}
