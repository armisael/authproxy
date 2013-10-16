package main

import (
	"flag"
	"github.com/gigaroby/authproxy/authserver"
	"github.com/gigaroby/authproxy/proxy"
	"github.com/vad/go-bunyan/bunyan"
	"net"
	"net/http"
	"strings"
	"time"
)

const PROXY_PORT = ":8080"

var (
	providerKey             string
	yesBroker               = flag.Bool("yes", false, "use the yes broker (instead of 3scale)")
	providerKeyAlternatives = flag.String("3scale-provider-key-alt", "", "comma separated pairs (elements are column separated) of label:providerKey, used in API calls")
	serviceFile             = flag.String("services-file", "/etc/authproxy/services.json", "file to load services from")
	backendsFile            = flag.String("backends-file", "/etc/authproxy/backends.json", "file to load backends from")
	adminPath               = flag.String("admin", "admin", "change the admin path (it will be on '/THIS_VALUE/'")
	timeout                 = time.Duration(2) * time.Second // this should be configurable for every service
	logger                  = bunyan.NewLogger("authproxy.main")
)

func dialTimeout(network, addr string) (net.Conn, error) {
	return net.DialTimeout(network, addr, timeout)
}

func main() {
	flag.StringVar(&providerKey, "3scale-provider-key", "", "3scale provider key")
	flag.Parse()

	var broker proxy.AuthenticationBroker
	if *yesBroker {
		broker = &proxy.YesBroker{}
	} else {
		if providerKey == "" {
			logger.Fatal("Missing parameter --3scale-provider-key")
		}

		pkAlts := *providerKeyAlternatives
		pkAltsMap := make(map[string]string)
		if pkAlts != "" {
			for _, pairString := range strings.Split(pkAlts, ",") {
				pair := strings.Split(pairString, ":")
				if len(pair) != 2 {
					logger.Fatal("Invalid column separated string (should be 2 elements): ", pairString)
				}
				pkAltsMap[pair[0]] = pair[1]
			}
		}
		broker = proxy.NewThreeScaleBroker(providerKey, pkAltsMap, nil)
	}

	// TODO[vad]: check if files exist

	transport := &http.Transport{
		Dial: dialTimeout,
	}

	proxyHandler := proxy.NewProxyHandler(broker, transport, *serviceFile, *backendsFile)
	authServer := authserver.NewHandle(broker, proxyHandler, *adminPath)

	server := &http.Server{
		Addr:    PROXY_PORT,
		Handler: authServer,
	}

	logger.Info(server.ListenAndServe())
}
