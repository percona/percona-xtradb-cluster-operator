/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// A small utility program to lookup hostnames of endpoints in a service.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	pollPeriod = 1 * time.Second
	waitPeriod = 30 * time.Second
)

var (
	onChange  = flag.String("on-change", "", "Script to run on change, must accept a new line separated list of peers via stdin.")
	onStart   = flag.String("on-start", "", "Script to run on start, must accept a new line separated list of peers via stdin.")
	svc       = flag.String("service", "", "Governing service responsible for the DNS records of the domain this pod is in.")
	namespace = flag.String("ns", "", "The namespace this pod is running in. If unspecified, the POD_NAMESPACE env var is used.")
	domain    = flag.String("domain", "", "The Cluster Domain which is used by the Cluster, if not set tries to determine it from /etc/resolv.conf file.")
	protocol  = flag.String("protocol", "", "The protocol used for SRV lookups. The default value is empty string. Acceptable values are also tcp and udp.")
)

func lookup(svcName, protocol string) (sets.String, error) {
	endpoints := sets.NewString()

	// setting 'service' a value only when protocol is non-empty
	// because `LookupSRV` internally has a condition on its default
	// resolver for checking if service == "" && proto == "".
	service := ""
	if protocol != "" {
		service = "mysql"
	}
	_, srvRecords, err := net.LookupSRV(service, protocol, svcName)
	if err != nil {
		return endpoints, err
	}
	for _, srvRecord := range srvRecords {
		// The SRV records ends in a "." for the root domain
		ep := fmt.Sprintf("%v", srvRecord.Target[:len(srvRecord.Target)-1])
		endpoints.Insert(ep)
	}
	return endpoints, nil
}

func shellOut(sendStdin, script string) {
	log.Printf("execing: %v with stdin: %v", script, sendStdin)
	// TODO: Switch to sending stdin from go
	out, err := exec.Command("bash", "-c", fmt.Sprintf("echo -e '%v' | %v", sendStdin, script)).CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to execute %v: %v, err: %v", script, string(out), err)
	}
	log.Print(string(out))
}

func main() {
	// Custom signal handler for SIGUSR1. This is needed because the
	// Docker image (percona/percona-xtradb-cluster-operator:$version-haproxy)
	// sets the STOPSIGNAL to SIGUSR1 to shutdown HAProxy gracefully.
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGUSR1)
	go func() {
		<-signalChan
		os.Exit(0)
	}()

	flag.Parse()

	ns := *namespace
	if ns == "" {
		ns = os.Getenv("POD_NAMESPACE")
	}
	log.Printf("Peer finder enter")
	var domainName string

	// If domain is not provided, try to get it from resolv.conf
	if *domain == "" {
		resolvConfBytes, err := os.ReadFile("/etc/resolv.conf")
		resolvConf := string(resolvConfBytes)
		if err != nil {
			log.Fatal("Unable to read /etc/resolv.conf")
		}

		var re *regexp.Regexp
		if ns == "" {
			// Looking for a domain that looks like with *.svc.**
			re, err = regexp.Compile(`\A(.*\n)*search\s{1,}(.*\s{1,})*(?P<goal>[a-zA-Z0-9-]{1,63}.svc.([a-zA-Z0-9-]{1,63}\.)*[a-zA-Z0-9]{2,63})`)
		} else {
			// Looking for a domain that looks like svc.**
			re, err = regexp.Compile(`\A(.*\n)*search\s{1,}(.*\s{1,})*(?P<goal>svc.([a-zA-Z0-9-]{1,63}\.)*[a-zA-Z0-9]{2,63})`)
		}
		if err != nil {
			log.Fatalf("Failed to create regular expression: %v", err)
		}

		groupNames := re.SubexpNames()
		result := re.FindStringSubmatch(resolvConf)
		for k, v := range result {
			if groupNames[k] == "goal" {
				if ns == "" {
					// Domain is complete if ns is empty
					domainName = v
				} else {
					// Need to convert svc.** into ns.svc.**
					domainName = ns + "." + v
				}
				break
			}
		}
		log.Printf("Determined Domain to be %s", domainName)
	} else {
		domainName = strings.Join([]string{ns, "svc", *domain}, ".")
	}

	if *svc == "" || domainName == "" || (*onChange == "" && *onStart == "") {
		log.Fatalf("Incomplete args, require -on-change and/or -on-start, -service and -ns or an env var for POD_NAMESPACE.")
	}

	script := *onStart
	if script == "" {
		script = *onChange
		log.Printf("No on-start supplied, on-change %v will be applied on start.", script)
	}
	newPeers := sets.NewString()
	var err error

	isFirstUpdate := true
	lastChangeTime := time.Now()

	normalizedProtocol, err := normalizeAndValidateProtocol(protocol)
	if err != nil {
		log.Fatal(err)
	}
	proto := normalizedProtocol

	for peers := sets.NewString(); script != ""; time.Sleep(pollPeriod) {
		newPeers, err = lookup(*svc, proto)
		if err != nil {
			log.Printf("%v", err)

			if lerr, ok := err.(*net.DNSError); ok && lerr.IsNotFound {
				// Service not resolved - no endpoints, so reset peers list
				peers = sets.NewString()
				continue
			}
		}
		peerList := newPeers.List()
		sort.Strings(peerList)
		if strings.Join(peers.List(), ":") != strings.Join(newPeers.List(), ":") {
			if isFirstUpdate || time.Since(lastChangeTime) >= waitPeriod {
				log.Printf("Peer list updated\nwas %v\nnow %v", peers.List(), newPeers.List())
				shellOut(strings.Join(peerList, "\n"), script)
				peers = newPeers
				lastChangeTime = time.Now()
				isFirstUpdate = false
			} else {
				log.Printf("Ignoring peer list update, last change was %v ago", time.Since(lastChangeTime))
			}
		}
		script = *onChange
	}
	// TODO: Exit if there's no on-change?
	log.Printf("Peer finder exiting")
}

func normalizeAndValidateProtocol(protocol *string) (string, error) {
	// An empty protocol is accepted for backward compatibility,
	// as it was the implicit default before protocol configuration was introduced.
	// The current default is also an empty string, so it remains a valid value.
	if protocol == nil {
		return "", nil
	}

	proto := strings.TrimSpace(*protocol)

	// If the env var PEER_LIST_SRV_PROTOCOL does not exist in
	// the container env, 'protocol' will remain a literal string.
	if proto == "" || proto == "$(PEER_LIST_SRV_PROTOCOL)" {
		return "", nil
	}

	normalizedProtocol := strings.ToLower(proto)

	if normalizedProtocol != "udp" && normalizedProtocol != "tcp" {
		return "", fmt.Errorf("protocol must be either 'udp' or 'tcp', provided %v", *protocol)
	}
	return normalizedProtocol, nil
}
