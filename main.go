package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
)

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
		go func(url string, headers []string, values []string) {
			results <- doRequestWithHeader(url, headers, values)
		}(url, []string{GetNextFuzzyHeader()}, []string{GetNextFuzzyString()})
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

type Result struct {
	Url        string
	Headers    []string
	Values     []string
	StatusCode int
	Duration   time.Duration
	Err        error
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
	req, _ := http.NewRequest("GET", url, nil)
	for i := range headers {
		req.Header.Set(headers[i], values[i])
	}
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return Result{Err: err, Headers: headers, Values: values, Url: url}
	} else {
		defer resp.Body.Close()
		return Result{StatusCode: resp.StatusCode, Duration: time.Since(before), Headers: headers, Values: values, Url: url}
	}
}

var nextFuzzyString int = 0
var nextFuzzyHeader int = 0

func GetFuzzyStrings() []string {
	return []string{"Test",
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
}

func GetNextFuzzyString() string {
	s := GetFuzzyStrings()
	r := nextFuzzyString
	nextFuzzyString = (nextFuzzyString + 1) % len(s)
	return s[r]
}

func GetFuzzyHeaders() []string {
	return []string{"User-Agent",
		"Referer",
		"Cookie",
		"Content-Type",
		"Authorization",
		"Origin"}
}

func GetNextFuzzyHeader() string {
	s := GetFuzzyHeaders()
	r := nextFuzzyHeader
	nextFuzzyHeader = (nextFuzzyHeader + 1) % len(s)
	return s[r]
}
