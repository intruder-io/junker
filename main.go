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
}

func main() {
	conf := Config{}
	flag.BoolVarP(&conf.NoResolve, "no-resolve", "n", false, "Don't resolve domains, instead expect the input to be in the format <ip>,<url>")
	flag.IntVarP(&conf.Workers, "workers", "c", 10, "The number of concurrent wokers")
	flag.IntVarP(&conf.BatchSize, "batch-size", "b", 200, "The number of input lines to process at once, shuffling the tests for each")
	flag.StringSliceVarP(&conf.Methods, "methods", "m", []string{"POST"}, "HTTP method to test - to test multiple methods specify multiple times")
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

	// Misc. setup
	rand.Seed(time.Now().UTC().UnixNano())
	conf.Mutations = generateMutations()

	// Setup workers
	testsChan := make(chan SmuggleTest)
	resultsChan := make(chan SmuggleTest)
	errChan := make(chan error)

	workersWg := sync.WaitGroup{}
	workersWg.Add(conf.Workers)

	workers := make([]Worker, conf.Workers)
	headers := []string{
		"User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/42.0.2311.135 Safari/537.36 Edge/12.246",
		"Connection: Keep-Alive",
	}
	for i := range workers {
		workers[i] = Worker{
			Headers: headers,
			Timeout: 5 * time.Second,
		}
		go workers[i].Test(testsChan, resultsChan, errChan, workersWg.Done)
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
			if r.Result != TEST_RESULT_SAFE {
				j, err := json.Marshal(r)
				if err != nil {
					log.Printf("Error JSON marshalling test %v: %v\n", r, err)
				}
				fmt.Println(string(j))
			}
		}
		resultsWg.Done()
	}()

	// Read errors from workers
	errWg := sync.WaitGroup{}
	errWg.Add(1)
	go func() {
		for err := range errChan {
			log.Printf("Error: %v\n", err)
		}
		errWg.Done()
	}()

	// Wait for everything to finish
	workersWg.Wait()
	close(errChan)
	errWg.Wait()
	close(resultsChan)
	resultsWg.Wait()
}
