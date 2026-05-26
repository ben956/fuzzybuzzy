package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"
)

var (
	fuzzyStrings = []string{"Test",
		"\x00",
		"\x00\x00\x00",
		"\r\n",
		"\r\nX-Injected: evil",
		"\x7f",
		"",
		" ",
		"\t",
		"   ",
		"%s%s%s%s",
		"%d%d%d%d",
		"%x%x%x%x",
	}

	fuzzyHeaders = []string{"User-Agent",
		"Referer",
		"Cookie",
		"Content-Type",
		"Authorization",
		"Origin"}

	fuzzyStringIndex atomic.Uint64
	fuzzyHeaderIndex atomic.Uint64
)

type Result struct {
	Url        string
	Headers    []string
	Values     []string
	StatusCode int
	Duration   time.Duration
	Err        error
}

func nextFuzzerHeader() string {
	i := fuzzyHeaderIndex.Add(1) - 1
	return fuzzyHeaders[i%uint64(len(fuzzyHeaders))]
}

func nextFuzzyString() string {
	i := fuzzyStringIndex.Add(1) - 1
	return fuzzyStrings[i%uint64(len(fuzzyStrings))]
}

func main() {

	var url string
	var threadsCount int
	flag.StringVar(&url, "url", "", "The URL to test against")
	flag.IntVar(&threadsCount, "threads", 1, "How many threads to use for the test")
	flag.Parse()

	if url == "" {
		log.Fatal("Invalid URL")
	}
	if threadsCount <= 0 {
		log.Fatal("Invalid threads count:", threadsCount, "using default 1")
	}

	fmt.Println("url:", url)
	fmt.Println("threadsCount:", threadsCount)
	results := make(chan Result, threadsCount)

	/*	for i := 0; i < threadsCount; i++ {
			go func(url string) {
				results <- doRequest(url)
			}(url)
		}
	*/
	for i := 0; i < threadsCount; i++ {
		go func(headers []string, values []string) {
			results <- doRequestWithHeader(url, headers, values)
		}([]string{nextFuzzerHeader()}, []string{nextFuzzyString()})
	}

	for i := 0; i < threadsCount; i++ {
		result := <-results
		if result.Err != nil {
			fmt.Println("url:", url, " error:", result.Err)
		} else {
			fmt.Println("url:", url, "status:", result.StatusCode, "headers:", result.Headers, "values:", result.Values)
		}
	}
}

func doRequest(url string) Result {
	start := time.Now()
	resp, err := http.Get(url)
	duration := time.Since(start)
	if err != nil {
		return Result{Err: err, Duration: duration}
	}
	defer resp.Body.Close()
	return Result{StatusCode: resp.StatusCode, Duration: duration}
}

func doRequestWithHeader(url string, headers []string, values []string) Result {
	before := time.Now()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return Result{Err: err, Url: url, Headers: headers, Values: values}
	}
	for i, header := range headers {
		req.Header.Set(header, values[i])
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return Result{Err: err, Headers: headers, Values: values, Url: url}
	}
	defer resp.Body.Close()
	return Result{StatusCode: resp.StatusCode, Duration: time.Since(before), Headers: headers, Values: values, Url: url}
}
