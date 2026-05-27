package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
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

func nextFuzzyHeader() string {
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
	FuzzyHeaderTest(threadsCount, url)
}

func FuzzyHeaderTest(threadsCount int, url string) {
	results := make(chan Result, threadsCount)

	/*	for i := 0; i < threadsCount; i++ {
			go func(url string) {
				results <- doRequest(url)
			}(url)
		}
	*/
	for range threadsCount {
		go func(headers []string, values []string) {
			results <- doRequestWithHeader(url, headers, values)
			//it's ok to put nextFuzzerHeader and nextFuzzyString here
		}([]string{nextFuzzyHeader()}, []string{nextFuzzyString()})
	}

	for range threadsCount {
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

func doRequestWithHeader(rawUrl string, headers []string, values []string) Result {
	u, err := url.Parse(rawUrl)
	if err != nil {
		return Result{Url: rawUrl, Err: err, Headers: headers, Values: values}
	}
	host := u.Host
	if !strings.Contains(host, ":") {
		host += ":443"
	}
	conn, err := net.DialTimeout("tcp", host, 5*time.Second)
	if err != nil {
		return Result{Url: rawUrl, Err: err, Headers: headers, Values: values}
	}
	defer conn.Close()

	var sb strings.Builder
	fmt.Fprintf(&sb, "GET %s HTTP/1.1\r\n", u.RequestURI())
	fmt.Fprintf(&sb, "HOST: %s\r\n", u.Hostname())

	for i, header := range headers {
		fmt.Fprintf(&sb, "%s: %s\r\n", header, values[i])
	}
	sb.WriteString("Connection: close\r\n\r\n")
	log.Println("REQUEST:\n", sb.String())
	before := time.Now()
	conn.Write([]byte(sb.String()))

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)

	if err != nil {
		return Result{Err: err, Headers: headers, Values: values, Url: rawUrl}
	}
	defer resp.Body.Close()
	respDump, _ := httputil.DumpResponse(resp, false)
	log.Printf("RESPONSE: \n%s", respDump)
	return Result{StatusCode: resp.StatusCode, Duration: time.Since(before), Headers: headers, Values: values, Url: rawUrl}
}
