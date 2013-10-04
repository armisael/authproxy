package authserver

import (
	"bytes"
	"io"
	"io/ioutil"
	// "net/http"
	"net/http/httptest"
	"testing"
)

const (
	input = `
	       Besame, besame mucho,
	       Como si fuera esta noche la última vez,
	       Besame, besame mucho,
	       Que tengo miedo a perderte, perderte despues`
)

func TestLimitAndBufferBodyDoesWork(t *testing.T) {
	rw := httptest.NewRecorder()
	body := ioutil.NopCloser(bytes.NewReader([]byte(input)))

	out, err := limitAndBufferBody(rw, body, 1000)

	if err != nil {
		t.Error("Short text, high limit: should work, but it doesn't!")
	}

	read := make([]byte, len(input))
	out.Read(read)

	if string(read) != input {
		t.Error("Body corrupted by limitAndBufferBody. Expected:", input, "\nGot: ", string(read))
	}

	seeker, ok := out.(io.ReadSeeker)
	if !ok {
		t.Error("It should return a ReadSeeker")
	}
	seeker.Seek(0, 0)

	readAfterSeek := make([]byte, len(input))
	seeker.Read(readAfterSeek)

	if string(readAfterSeek) != input {
		t.Error("Seek() does not work as expected. Should be:", input, "\nGot: ", string(read))
	}
}

func TestLimitAndBufferBodyDoesLimit(t *testing.T) {
	rw := httptest.NewRecorder()
	body := ioutil.NopCloser(bytes.NewReader([]byte(input)))

	_, err := limitAndBufferBody(rw, body, 20)

	if err == nil {
		t.Error("Limit is not working")
	}
}

// func TestLimitAndBufferCanSeek(t *testing.T) {
// 	rw := httptest.NewRecorder()
// 	body := ioutil.NopCloser(bytes.NewReader([]byte(input)))

// 	out, err := limitAndBufferBody(rw, body, 1000)

// 	if err != nil {
// 		t.Error("Limit is not working")
// 	}
// }
