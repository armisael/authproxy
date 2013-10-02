package admin

import (
	"encoding/json"
	"github.com/gigaroby/authproxy/proxy"
	"net/http"
	"strconv"
)

type CreditsJson struct {
	CreditsLeft int    `json:"creditsLeft"`
	NextReset   string `json:"nextReset"`
}

type responseJson struct {
	Data    *CreditsJson `json:"data,omitempty"`
	Error   bool         `json:"error"`
	Message string       `json:"message"`
	code    string       `json:"code"`
}

type CreditsHandle struct {
	Broker *proxy.ThreeScaleBroker
}

func (h *CreditsHandle) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	query := req.URL.Query()

	appId := query.Get("$app_id")
	res := &responseJson{}

	// TODO[vad]: refactor this jungle using a wrapper function
	if appId == "" {
		res.Error = true
		res.Message = "Missing parameter $app_id"
	} else {
		_, msg, err := h.Broker.DoAuthenticate(appId, "")

		if err != nil {
			res.Error = true
			res.Message = "Error connecting to the authentication backend"
		} else {
			hits, err := strconv.Atoi(msg["creditsLeft"])

			if err != nil {
				res.Error = true
				res.Message = "Bad response from the authentication backend"
			} else {
				data := &CreditsJson{CreditsLeft: hits / proxy.ThreeScaleHitsMultiplier, NextReset: msg["creditsReset"]}
				res.Data = data
			}
		}
	}

	if res.Error {
		rw.WriteHeader(400)
	} else {
		rw.WriteHeader(200)
	}

	out, _ := json.Marshal(res)

	rw.Write(out)
}
