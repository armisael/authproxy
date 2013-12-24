package main

import (
	"crypto/tls"
	"flag"
	"github.com/gigaroby/authproxy/authserver"
	"github.com/gigaroby/authproxy/proxy"
	log "github.com/gigaroby/gopherlog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const PROXY_PORT = ":8080"

var (
	providerKey             string
	yesBroker               = flag.Bool("yes", false, "use the yes broker (instead of 3scale)")
	enableProfiler          = flag.Bool("profile", false, "Enable the profiler")
	providerKeyAlternatives = flag.String("3scale-provider-key-alt", "", "comma separated pairs (elements are column separated) of label:providerKey, used in API calls")
	serviceFile             = flag.String("services-file", "/etc/authproxy/services.json", "file to load services from")
	backendsFile            = flag.String("backends-file", "/etc/authproxy/backends.json", "file to load backends from")
	adminPath               = flag.String("admin", "admin", "change the admin path (it will be on '/THIS_VALUE/'")
	sentryDSN               = flag.String("sentry-dsn", "", "set the sentry dsn to be used for logging purposes")
	skipTLSVerify           = flag.Bool("skip-tls-verify", false, "skip the TLS check while connecting to backends")
	timeout                 = time.Duration(2) * time.Second // this should be configurable for every service
)

func setupLogging() *log.Logger {
	logger := log.GetLogger("authproxy.main")
	bunyanHandler := &log.BunyanHandler{Out: os.Stdout}
	log.RegisterHandler(bunyanHandler, log.DEBUG)
	if *sentryDSN != "" {
		ravenHandler := log.NewRavenHandler("authproxy", *sentryDSN)
		log.RegisterHandler(ravenHandler, log.ERROR)
	} else {
		log.Warning("sentry logging disabled. dsn was not provided")
	}
	return logger
}

func dialTimeout(network, addr string) (net.Conn, error) {
	return net.DialTimeout(network, addr, timeout)
}

func main() {
	flag.StringVar(&providerKey, "3scale-provider-key", "", "3scale provider key")
	flag.Parse()

	logger := setupLogging()

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
	if *skipTLSVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	proxyHandler := proxy.NewProxyHandler(broker, transport, *serviceFile, *backendsFile)
	authServer := authserver.NewHandle(broker, proxyHandler, *adminPath, *enableProfiler)

	server := &http.Server{
		Addr:    PROXY_PORT,
		Handler: authServer,
	}

	logger.Info(server.ListenAndServe())
}
