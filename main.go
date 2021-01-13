package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	flag "github.com/spf13/pflag"
)

type Config struct {
	// NoResolve determines whether to resolve domain names, or expect the input in the format <ip>,<url>
	NoResolve bool

	// The number of concurrent workers
	Workers int

	// The number of input lines to buffer and shuffle the tests for
	BatchSize int

	// The mutations to test
	Mutations map[string]string

	// The HTTP methods to test
	Methods []string

	// The file to output to
	OutFilename string

	// The timeout to use for HTTP requests
	Timeout time.Duration

	// The extra headers to include in HTTP requests
	Headers []string
}

func main() {
	conf := Config{}
	flag.BoolVarP(&conf.NoResolve, "no-resolve", "n", false, "Don't resolve domains, instead expect the input to be in the format <ip>,<url>")
	flag.IntVarP(&conf.Workers, "workers", "c", 10, "The number of concurrent wokers")
	flag.IntVarP(&conf.BatchSize, "batch-size", "b", 200, "The number of input lines to process at once, shuffling the tests for each")
	flag.StringSliceVarP(&conf.Methods, "methods", "m", []string{"POST"}, "HTTP method to test - to test multiple methods specify multiple times")
	flag.StringVarP(&conf.OutFilename, "output", "o", "junker.json", "The file to output results to, in JSON format - use '-' for stdout")
	flag.DurationVarP(&conf.Timeout, "timeout", "t", 5*time.Second, "The timeout value to use for HTTP requests")
	flag.StringSliceVarP(&conf.Headers, "headers", "H", []string{}, "Extra headers to include in requests - to add multiple headers specify multiple times")
	flag.Parse()

	// Input reading
	var scanner *bufio.Scanner
	if flag.NArg() == 1 {
		filename := flag.Arg(0)
		f, err := os.Open(filename)
		if err != nil {
			fmt.Printf("Failed to open intput file: %v\n", err)
			os.Exit(1)
		}
		scanner = bufio.NewScanner(f)
	} else {
		scanner = bufio.NewScanner(os.Stdin)
	}

	// Output setup
	var logger *log.Logger
	if conf.OutFilename == "-" {
		logger = log.New(os.Stdout, "", 0)
	} else {
		f, err := os.OpenFile(conf.OutFilename, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			fmt.Printf("Failed to open output file: %v\n", err)
			os.Exit(1)
		}
		logger = log.New(f, "", 0)
	}

	// Allow header overrriding from command line
	uaGiven := false
	connGiven := false
	for _, h := range conf.Headers {
		if strings.HasPrefix(h, "User-Agent:") {
			uaGiven = true
		} else if strings.HasPrefix(h, "Connection:") {
			connGiven = true
		}
	}

	if !uaGiven {
		conf.Headers = append(conf.Headers, "User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/42.0.2311.135 Safari/537.36 Edge/12.246")
	}

	if !connGiven {
		conf.Headers = append(conf.Headers, "Connection: Close")
	}

	// Misc. setup
	rand.Seed(time.Now().UTC().UnixNano())
	conf.Mutations = generateMutations()

	// Setup workers
	testsChan := make(chan SmuggleTest)
	resultsChan := make(chan SmuggleTest)

	workersWg := sync.WaitGroup{}
	workersWg.Add(conf.Workers)

	workers := make([]Worker, conf.Workers)
	for i := range workers {
		workers[i] = Worker{
			Headers: conf.Headers,
			Timeout: conf.Timeout,
		}
		go workers[i].Test(testsChan, resultsChan, workersWg.Done)
	}

	// Feed input to workers
	go func() {
		done := false
		for !done {
			// Fill up the batch
			batch := make([]string, 0)
			i := 0
			for i < conf.BatchSize && scanner.Scan() {
				batch = append(batch, scanner.Text())
				i++
			}

			if i < conf.BatchSize {
				done = true
			}

			if len(batch) == 0 {
				done = true
				break
			}

			// Generate tests for the batch
			// These tests are not shuffled, and should be chosen from at random
			tests := make([]SmuggleTest, 0)
			for _, line := range batch {
				// Input parsing and resolution
				var ips []net.IP
				var u *url.URL
				if conf.NoResolve {
					parts := strings.Split(line, ",")
					if len(parts) != 2 {
						log.Printf("Invalid input line: %s\n", line)
						continue
					}

					ip := net.ParseIP(parts[0])
					ips = []net.IP{ip}
					var err error
					u, err = url.Parse(parts[1])
					if err != nil {
						log.Printf("Failed to parse URL: %v\n", parts[1])
						continue
					}
				} else {
					var err error
					u, err = url.Parse(line)
					if err != nil {
						log.Printf("Failed to parse URL: %v\n", line)
						continue
					}
					ips, err = net.LookupIP(u.Hostname())
					if err != nil {
						log.Printf("Failed to resolve host: %v\n", u.Hostname())
						continue
					}
				}

				// Test generation
				for _, ip := range ips {
					keys := make([]string, 0)
					for k := range conf.Mutations {
						keys = append(keys, k)
					}
					for i := range keys {
						for j := i + 1; j < len(keys); j++ {
							m1 := conf.Mutations[keys[i]]
							m2 := conf.Mutations[keys[j]]

							t := SmuggleTest{
								Url:       u,
								IP:        ip,
								Method:    "POST",
								Mutations: [2]string{m1, m2},
							}
							tests = append(tests, t)
						}
					}
				}
			}

			// Distribute the batch to the workers, selecting randomly from the tests
			for len(tests) > 0 {
				i := rand.Intn(len(tests))
				t := tests[i]
				tests = append(tests[:i], tests[i+1:]...)

				testsChan <- t
			}
		}

		close(testsChan)
	}()

	// Read results from workers
	resultsWg := sync.WaitGroup{}
	resultsWg.Add(1)
	go func() {
		for r := range resultsChan {
			j, err := json.Marshal(r)
			if err != nil {
				log.Printf("Error JSON marshalling test %v: %v\n", r, err)
			}
			logger.Println(string(j))
		}
		resultsWg.Done()
	}()

	// Wait for everything to finish
	workersWg.Wait()
	close(resultsChan)
	resultsWg.Wait()
}
