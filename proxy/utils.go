package proxy

import (
	"net/url"
	"time"
)

type Service url.URL

func (s Service) String() string {
	u := url.URL(s)
	return u.String()
}

func attempt(maxRetray int, retrayDelay time.Duration, attemptFunc func() error) error {
	var i int = 1
	for {
		err := attemptFunc()
		if err != nil {
			if i < maxRetray {
				time.Sleep(retrayDelay)
				i++
				continue
			}
			return err
		} else {
			return nil
		}
	}
}
