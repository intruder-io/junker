package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type TestResult string

const (
	TEST_RESULT_R1_EQUAL_BASELINE = "r1=b"
	TEST_RESULT_R2_EQUAL_BASELINE = "r2=b"
	TEST_RESULT_R1_EQUAL_R2       = "r1=r2"
	TEST_RESULT_VULNERABLE        = "vulnerable"
	TEST_RESULT_ERROR             = "error"
	TEST_RESULT_TIMEOUT           = "timeout"
)

type SmuggleTest struct {
	// The URL to test
	Url *url.URL

	// The IP address to target
	IP net.IP

	// The HTTP method to test
	Method string

	// The mutations to test, as format strings
	Mutations [2]string

	// The requests sent in testing
	Requests [3][]byte

	// The responses received from the server from Requests
	Responses [3][]byte

	// The result of the test
	Result TestResult

	// Any error that caused testing to abort
	Error error

	// Time checks strarted
	Start time.Time

	// Times checks finished
	End time.Time

	// How many times has this test been performed?
	Runs int
}

type Worker struct {
	// Headers to include in the request
	Headers []string

	// The time to wait for a request
	Timeout time.Duration

	// How many runs of each test to do
	Runs int
}

func (w Worker) Test(tests <-chan SmuggleTest, results chan<- SmuggleTest, done func()) {
	for t := range tests {
		r := w.requestBase(t.Url, t.Mutations, t.Method)

		t.Runs = 0
		t.Requests[0] = []byte(fmt.Sprintf(r, "0", "0"))
		t.Requests[1] = []byte(fmt.Sprintf(r, "z", "0"))
		t.Requests[2] = []byte(fmt.Sprintf(r, "0", "z"))

		t.Start = time.Now()

		// Send requests
		var err error
		for i := 0; i < 3*w.Runs; i++ {
			var timeout bool
			j := i % 3
			if j == 0 {
				t.Runs++
			}

			t.Responses[j], err, timeout = w.sendRequest(t.IP, t.Requests[j], t.Url, w.Timeout)
			if err != nil {
				t.Error = err
				t.Result = TEST_RESULT_ERROR
				break
			}

			if timeout {
				t.Error = fmt.Errorf("timeout sending request to %s", t.Url.String())
				t.Result = TEST_RESULT_TIMEOUT
				break
			}

			// Do the checking as we go to prevent from sending more requests than
			// necessary
			if j == 1 && compareResponses(t.Responses[0], t.Responses[1]) {
				t.Result = TEST_RESULT_R1_EQUAL_BASELINE
				break
			}

			if j == 2 {
				if compareResponses(t.Responses[0], t.Responses[2]) {
					t.Result = TEST_RESULT_R2_EQUAL_BASELINE
					break
				} else if compareResponses(t.Responses[1], t.Responses[2]) {
					t.Result = TEST_RESULT_R1_EQUAL_R2
					break
				} else {
					t.Result = TEST_RESULT_VULNERABLE
				}
			}
		}

		t.End = time.Now()
		results <- t
	}

	done()
}

// sendRequest sends the specified request, but doesn't try to parse the response,
// and instead just returns it
// Adapted from smuggles
func (w Worker) sendRequest(ip net.IP, req []byte, u *url.URL, timeout time.Duration) (resp []byte, err error, isTimeout bool) {
	var cerr error
	var conn io.ReadWriteCloser

	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	target := net.JoinHostPort(ip.String(), port)
	if u.Scheme == "https" {
		conf := &tls.Config{InsecureSkipVerify: true}
		conn, cerr = tls.DialWithDialer(&net.Dialer{
			Timeout: timeout,
		}, "tcp", target, conf)

	} else {
		d := net.Dialer{Timeout: timeout}
		conn, cerr = d.Dial("tcp", target)
	}

	if cerr != nil {
		err = cerr
		return
	}

	_, err = conn.Write(req)
	if err != nil {
		return
	}

	// See if we can read before the timeout
	c := make(chan []byte)
	e := make(chan error)
	go func() {
		r, err := ioutil.ReadAll(conn)
		if err != nil {
			e <- err
		} else {
			c <- r
		}
	}()

	select {
	case resp = <-c:
	case err = <-e:
	case <-time.After(timeout):
		isTimeout = true
		conn.Close()
	}

	return
}

// requestBase returns the base for test requests. This base contains a %s marker in the location of each of the two mutationHeaders to fill in with a value
func (w Worker) requestBase(u *url.URL, mutationHeaders [2]string, method string) string {
	path := "/"
	if u.Path != "" {
		path = u.Path
	}

	r := fmt.Sprintf("%s %s HTTP/1.1\r\n", method, path)
	r += fmt.Sprintf("Host: %s\r\n", u.Hostname())
	r += mutationHeaders[0] + "\r\n"
	r += mutationHeaders[1] + "\r\n"

	if w.Headers != nil {
		for _, h := range w.Headers {
			r += h + "\r\n"
		}
	}
	r += "\r\n"

	return r
}

// compareResponses returns whether HTTP responses r1 and r2 are the same. Responses are
// considered to be different if one of the following is true:
//  - the status lines are not equal
//  - the length of the response bodies differs by more than 20%
func compareResponses(r1, r2 []byte) bool {
	// Check status lines
	s1 := strings.Split(string(r1), "\n")[0]
	s2 := strings.Split(string(r2), "\n")[0]
	if s1 != s2 {
		return false
	}

	// Response lengths
	l1 := getResponseLength(r1)
	l2 := getResponseLength(r2)
	l1f := float64(l1)
	l2f := float64(l2)
	if l1f < 0.8*l2f || l1f > 1.2*l2f {
		return false
	}

	return true
}

// getResponseLength gets the length of an HTTP response stored in a byte array
func getResponseLength(r []byte) int64 {
	b := bytes.NewReader(r)
	reader := bufio.NewReader(b)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		return -1
	}

	return resp.ContentLength
}
