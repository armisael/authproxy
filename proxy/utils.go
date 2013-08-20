package proxy

import (
	"time"
)

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
