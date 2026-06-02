package main

// Package name, the package is effectively folder, all files within a folder share the same package name. No inheritance.

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
)

// aboves are imports, again, they stand for folder name.

var (
	// the var{} could  be used by shared variables
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
	// package only scope

	fuzzyHeaders = []string{"User-Agent",
		"Referer",
		"Cookie",
		"Content-Type",
		"Authorization",
		"Origin"}

	fuzzyStringIndex atomic.Uint64
	fuzzyHeaderIndex atomic.Uint64
)

var treeLevel uint
var MAX_TREE_LEVEL uint = 10

type Result struct {
	Url        string
	Headers    []string
	Values     []string
	StatusCode int
	Duration   time.Duration
	Err        error
	Body       io.ReadCloser
}

type InputInfo struct {
	Name  string
	Type  string
	Value string
}

type FormInfo struct {
	Action string
	Method string
	Inputs []InputInfo
}

type PageInfo struct {
	Url   string
	Links []string
	Form  []FormInfo
	Input []InputInfo
}

// data container

func nextFuzzyHeader() string {
	i := fuzzyHeaderIndex.Add(1) - 1
	// declare local var
	return fuzzyHeaders[i%uint64(len(fuzzyHeaders))]
	// uint64(x) type conversation
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
	// Golang way to read parameters

	if url == "" {
		log.Fatal("Invalid URL")
	}
	if threadsCount <= 0 {
		log.Fatal("Invalid threads count:", threadsCount, "using default 1")
	}

	fmt.Println("url:", url)
	fmt.Println("threadsCount:", threadsCount)
	//FuzzyHeaderTest(threadsCount, url)
	r := doRequest(url)
	if r.Err != nil {
		log.Fatal("Fail to connect", url)
	}
	n, err := parseHtml(r.Body)
	if err != nil {
		log.Fatal("Fail to parse. err:", err)
	}
	walk(n, 1)
}

func FuzzyHeaderTest(threadsCount int, url string) {
	results := make(chan Result, threadsCount)
	// make a channel

	for range threadsCount {
		// this for range is a better way of i:=0; i < threadsCount; i++ {}
		go func(headers []string, values []string) {
			// go function is start a Golang thread(not computer thread). Be careful about how you pass in the parameters.
			results <- doRequestWithHeader(url, headers, values)
			// this send doRequestWithHeader result back to the channel
			//it's ok to put nextFuzzerHeader and nextFuzzyString here
		}([]string{nextFuzzyHeader()}, []string{nextFuzzyString()})
	}

	// fmt.Println("------Results------")
	for range threadsCount {
		result := <-results
		// read from channel
		if result.Err != nil {
			fmt.Println("url:", url, " error:", result.Err)
		} else {
			fmt.Println("url:", url, "status:", result.StatusCode, "headers:", result.Headers, "values:", result.Values)
		}
	}
}

func parseHtml(body io.ReadCloser) (*html.Node, error) {
	defer body.Close()
	return html.Parse(body)
}

// Non-recrusively find Input under the input node and return array of InputInfo
func getInputInfo(n *html.Node) []InputInfo {
	ret := []InputInfo{}
	if n == nil {
		return ret
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "input" {
			var name, t, v string
			for _, a := range c.Attr {
				switch a.Key {
				case "name":
					name = a.Val
				case "type":
					t = a.Val
				case "value":
					v = a.Val
				}
			}
			ret = append(ret, InputInfo{Type: t, Name: name, Value: v})
		}
	}
	return ret
}

// extract FormInfo from the input n, the n should be a Form
func getFormInfo(n *html.Node) (FormInfo, error) {
	var ret FormInfo
	if n == nil || n.Data != "form" {
		return ret, errors.New("The input is not a form")
	}
	var action, method string
	for _, attr := range n.Attr {
		switch attr.Key {
		case "action":
			action = attr.Val
		case "method":
			method = attr.Val

		}
	}
	return FormInfo{Method: method, Action: action, Inputs: getInputInfo(n)}, nil

}

func parsePageInfo(n *html.Node, url string) PageInfo {

	var ret PageInfo
	var walk func(m *html.Node, level uint)

	walk = func(n *html.Node, level uint) {
		if level >= MAX_TREE_LEVEL {
			//fmt.Println("reach max level ", level)
			return
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "a":
				for _, a := range n.Attr {
					if a.Key == "href" {
						fmt.Println(strings.Repeat(" ", int(level)), "level:", level, "href:", a.Val)
						ret.Links = append(ret.Links, a.Val)
					}
				}

			case "form":
				form, error := getFormInfo(n)
				if error != nil {
					fmt.Println("Fail to process form:", n)
				}
				ret.Form = append(ret.Form, form)

			case "input":
				inputs := getInputInfo(n)
				ret.Input = append(ret.Input, inputs...)

			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, level+1)
		}
	}
	return ret

}

func doRequest(url string) Result {
	start := time.Now()
	resp, err := http.Get(url)
	duration := time.Since(start)
	if err != nil {
		return Result{Err: err, Duration: duration}
	}
	return Result{StatusCode: resp.StatusCode, Duration: duration, Body: resp.Body}
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
	// similar with finally, be careful about the scope

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
