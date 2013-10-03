package admin

import (
	"encoding/json"
	"github.com/gigaroby/authproxy/proxy"
	"github.com/vad/go-bunyan/bunyan"
	"net/http"
	"strconv"
)

var (
	logger = bunyan.NewLogger("authproxy.admin")
)

type CreditsJson struct {
	CreditsLeft int    `json:"creditsLeft"`
	NextReset   string `json:"nextReset"`
}

type responseJson struct {
	Data    *CreditsJson `json:"data,omitempty"`
	Error   bool         `json:"error"`
	Message string       `json:"message"`
	Code    string       `json:"code,omitempty"`
	Status  int          `json:"status"`
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
		_, msg, err := h.Broker.DoAuthenticate(appId, "", "")

		if err != nil {
			logger.Info("Error connecting to the authentication backend: ", err.Error())
			res.Error = true
			res.Message = "Error connecting to the authentication backend"
			res.Code = "api.auth.error"
		} else {
			hits, err := strconv.Atoi(msg["creditsLeft"])

			if err != nil {
				res.Error = true
				res.Message = "Bad response from the authentication backend"
				res.Code = "api.auth.error"
			} else {
				data := &CreditsJson{CreditsLeft: hits / proxy.ThreeScaleHitsMultiplier, NextReset: msg["creditsReset"]}
				res.Data = data
			}
		}
	}

	if res.Error {
		res.Status = 400
	} else {
		res.Status = 200
	}
	rw.WriteHeader(res.Status)

	out, _ := json.Marshal(res)

	rw.Write(out)
}
