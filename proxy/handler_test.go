package proxy

import (
	"encoding/json"
	. "github.com/gigaroby/authproxy/testutils"
	. "github.com/smartystreets/goconvey/convey"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCopyHeader(t *testing.T) {
	header := "X-DL-cucu"
	headerCamel := "X-DL-Cucu"
	src := http.Header{header: {"4"}}
	dst := make(http.Header)

	copyHeader(dst, src)

	// accessing the Header with square brakets notation preserves the case
	if len(dst[header]) != 1 || dst[header][0] != "4" {
		if len(dst[headerCamel]) == 1 && dst[headerCamel][0] == "4" {
			t.Error("The tested function does not preserve case, it converts to CamelCase")

		} else {
			t.Error("The header has not been copied correctly. Expected", []string{"4"}, "got", dst[header])
		}
	}
}

func TestProxyHandlerPaths(t *testing.T) {
	trans := &RecordTransport{}
	proxy := NewProxyHandler(nil, trans, "test_data/services.json", "test_data/backends.json")

	Convey("Given a user that queries an API endpoint", t, func() {
		Convey("When he asks a not existent endpoint", func() {
			rw := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "http://localhost/service100/v1/", nil)
			proxy.ServeHTTP(rw, req)

			Convey("He gets a 404", func() {
				So(rw.Code, ShouldEqual, 404)
			})
			Convey("He gets a valid JSON response", func() {
				var v map[string]interface{}

				content, _ := ioutil.ReadAll(rw.Body)
				err := json.Unmarshal(content, &v)

				So(err, ShouldBeNil)
				So(v["error"], ShouldEqual, true)
				So(v["code"], ShouldEqual, "error.notFound")
			})
		})

		Convey("When he GETs the right URL", func() {
			Convey("He gets proxied to the right service", func() {
				rw := httptest.NewRecorder()
				req, _ := http.NewRequest("GET", "http://localhost/service1/v1/", nil)
				proxy.ServeHTTP(rw, req)
				url := trans.LastRequest.URL
				So(url.Host, ShouldEqual, "example.com")
				So(url.Path, ShouldEqual, "/service1")
			})
		})

		Convey("When he forgets the trailing /", func() {
			Convey("He gets proxied to the right service", func() {
				rw := httptest.NewRecorder()
				req, _ := http.NewRequest("GET", "http://localhost/service1/v1", nil)
				proxy.ServeHTTP(rw, req)
				url := trans.LastRequest.URL
				So(url.Host, ShouldEqual, "example.com")
				So(url.Path, ShouldEqual, "/service1")
			})
		})

		Convey("When he GETs a service that is under HTTPS", func() {
			Convey("He gets proxied to the right service", func() {
				rw := httptest.NewRecorder()
				req, _ := http.NewRequest("GET", "http://localhost/service2/v1", nil)
				proxy.ServeHTTP(rw, req)
				url := trans.LastRequest.URL
				So(url.Scheme, ShouldEqual, "https")
				So(url.Host, ShouldEqual, "example.com")
				So(url.Path, ShouldEqual, "/service2")
			})
		})
	})
}

func TestProxyHandlerBackendErrors(t *testing.T) {

	Convey("Given a user that queries an API endpoint", t, func() {
		Convey("When he GETs a service with a backend that returns a 3xx", func() {
			trans := &FactoryTransport{Response: NewResponse(301, "")}
			proxy := NewProxyHandler(nil, trans, "test_data/services.json", "test_data/backends.json")

			Convey("He gets an error", func() {
				rw := httptest.NewRecorder()
				req, _ := http.NewRequest("GET", "http://localhost/service2/v1", nil)
				proxy.ServeHTTP(rw, req)
				So(rw.Code, ShouldEqual, 502)

				var v map[string]interface{}
				content, _ := ioutil.ReadAll(rw.Body)
				err := json.Unmarshal(content, &v)

				So(err, ShouldBeNil)
				So(v["error"], ShouldEqual, true)
				So(v["code"], ShouldEqual, "error.badGateway")
			})
		})
	})
}
