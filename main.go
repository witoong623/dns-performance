package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

const Iteration = 1000

var DNSServerAddr string

type Result struct {
	Domain  string
	Elapsed time.Duration
	RTT     time.Duration
	Answer  string
	Error   string
}

func main() {
	//iteration := flag.Int("--iteration", 2, "the number of times to repeat query per domain")
	dnsServer := flag.String("dns-server", "", "IPv4 address of the server")
	domainFile := flag.String("file", "", "the name of file contains domain name, 1 domain per line")
	flag.Parse()
	if *domainFile == "" {
		log.Fatalln("filename is required")
	}
	if *dnsServer == "" {
		log.Fatalln("DNS server address is required")
	}
	DNSServerAddr = net.JoinHostPort(*dnsServer, "53")

	file, err := os.Open(*domainFile)
	if err != nil {
		log.Fatal(err)
	}

	scanner := bufio.NewScanner(file)
	domainList := make([]string, 0, 50)
	for scanner.Scan() {
		domainList = append(domainList, scanner.Text())
	}
	log.Printf("there are %v domains\n", len(domainList))

	// Per query performance
	resultChan := make(chan Result, 10)
	resultStrings := make([]string, 0, Iteration*len(domainList))
	//resultList := make([]Result, 0, Iteration*len(domainList))
	go func() {
		for result := range resultChan {
			resultstring := fmt.Sprintf("%s,%d,%d,%s,%s\n", result.Domain, result.Elapsed, result.RTT, result.Answer, result.Error)
			resultStrings = append(resultStrings, resultstring)
			//resultList = append(resultList, result)
		}
	}()
	var wg sync.WaitGroup
	dispatcher := NewDispatcher(MaxWorker)
	dispatcher.Run(&wg)

	startTime := time.Now()
	for i := 0; i < Iteration; i++ {
		for _, domain := range domainList {
			msg := new(dns.Msg)
			msg.SetQuestion(dns.Fqdn(domain), dns.TypeA)
			job := Job{Msg: msg, ResultChan: resultChan}
			JobQueue <- job
		}
	}
	close(JobQueue)
	wg.Wait()
	endTime := time.Now()
	testTime := endTime.Sub(startTime)
	log.Printf("query testing done, it took %v\n", testTime)
	rfile, err := os.Create("result.csv")
	if err != nil {
		panic(err)
	}
	defer rfile.Close()

	for _, line := range resultStrings {
		rfile.WriteString(line)
	}
	log.Println("all done!")
	/* summaryResultMap := make(map[string]int)
	for _, result := range resultList {
		curVal, ok := summaryResultMap[result.Answer]
		if ok {
			curVal++
		} else {
			curVal = 1
		}
		summaryResultMap[result.Answer] = curVal
	}
	for ip, count := range summaryResultMap {
		log.Printf("IP %v has been returned %v times\n", ip, count)
	} */
}

func (j *Job) queryAndMesaure() {
	dnsClient := new(dns.Client)
	start := time.Now()
	res, rtt, err := dnsClient.Exchange(j.Msg, DNSServerAddr)
	end := time.Now()
	if err == nil {
		ip := strings.Split(res.Answer[0].String(), "\t")[4]
		j.ResultChan <- Result{Domain: j.Msg.Question[0].Name, Elapsed: end.Sub(start), RTT: rtt, Answer: ip}
	} else {
		j.ResultChan <- Result{Domain: j.Msg.Question[0].Name, Elapsed: end.Sub(start), RTT: rtt, Error: err.Error()}
	}
}
