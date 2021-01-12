package main

import (
	"fmt"
	"net"
	"net/url"
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

	// The names of the mutations to test
	Mutations [2]string
}

type Worker struct {
	// The scanner config
	Conf Config
}

func (w Worker) Test(tests <-chan SmuggleTest, results chan<- SmuggleTest, errors chan<- error, done func()) {
	for t := range tests {
		fmt.Println(t)
		time.Sleep(2 * time.Second)
	}

	done()
}
