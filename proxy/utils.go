package proxy

import (
	log "github.com/gigaroby/gopherlog"
	"net/url"
	"time"
)

var (
	logger = log.GetLogger("authproxy.utils")
)

type Service url.URL

func (s Service) String() string {
	u := url.URL(s)
	return u.String()
}

func attempt(maxRetray int, retrayDelay time.Duration, attemptFunc func() error) (err error) {
	i := 1
	for {
		err = attemptFunc()
		if err != nil {
			if i < maxRetray {
				time.Sleep(retrayDelay)
				i++
				continue
			}
			return
		} else {
			return
		}
	}
}
