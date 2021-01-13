package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/url"
	"strings"
	"time"
)

type TestResult string

const (
	TEST_RESULT_SAFE       = "SAFE"
	TEST_RESULT_VULNERABLE = "VULNERABLE"
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

	// The requests to be sent
	Requests [2][]byte

	// The responses received from the server
	Responses [2][]byte

	// The result of the test
	Result TestResult
}

type Worker struct {
	// Headers to include in the request
	Headers []string

	// The time to wait for a request
	Timeout time.Duration
}

func (w Worker) Test(tests <-chan SmuggleTest, results chan<- SmuggleTest, errors chan<- error, done func()) {
	for t := range tests {
		t.Requests = w.generateRequests(t.Url, t.Mutations, t.Method)

		// Send requests
		var err error
		for i := 0; i < 2; i++ {
			t.Responses[i], err, _ = w.sendRequest(t.IP, t.Requests[i], t.Url, w.Timeout)
			if err != nil {
				errors <- err
				continue
			}
		}

		// Compare
		if !compareResponses(t.Responses[0], t.Responses[1]) {
			t.Result = TEST_RESULT_VULNERABLE
		} else {
			t.Result = TEST_RESULT_SAFE
		}

		results <- t
	}

	done()
}

// sendRequest sends the specified request, but doesn't try to parse the response,
// and instead just returns it
// Taken from smuggles
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
	target := fmt.Sprintf("%s:%s", ip.String(), port)
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

// generateRequests generates the pair of requests used to test the service for CL.CL
// request smuggling. Different errors returned from both of these requests indicates
// that the service is vulnerable.
func (w Worker) generateRequests(u *url.URL, mutationHeaders [2]string, method string) [2][]byte {
	path := "/"
	if u.Path != "" {
		path = u.Path
	}

	req1 := fmt.Sprintf("%s %s HTTP/1.1\r\n", method, path)
	req1 += fmt.Sprintf("Host: %s\r\n", u.Hostname())
	req1 += mutationHeaders[0] + "\r\n"
	req1 += mutationHeaders[1] + "\r\n"

	if w.Headers != nil {
		for _, h := range w.Headers {
			req1 += h + "\r\n"
		}
	}
	req1 += "\r\n"

	// Formatting
	req2 := req1
	req1 = fmt.Sprintf(req1, "0", "z")
	req2 = fmt.Sprintf(req2, "z", "0")

	return [2][]byte{[]byte(req1), []byte(req2)}
}

// compareResponses returns whether HTTP responses r1 and r2 are the same. Responses are
// considered to be different if one of the following is true:
//  - the status lines are not equal
//  - the length of the responses differs by more than 20%
func compareResponses(r1, r2 []byte) bool {
	// Check status lines
	s1 := strings.Split(string(r1), "\n")[0]
	s2 := strings.Split(string(r2), "\n")[0]
	if s1 != s2 {
		return false
	}

	// Response lengths
	l1 := float64(len(r1))
	l2 := float64(len(r2))
	if l1 <= 0.8*l2 || l1 >= 1.2*l2 {
		return false
	}

	return true
}
