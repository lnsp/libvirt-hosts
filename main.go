// Copyright 2020 Lennart Espe
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/digitalocean/go-libvirt"
	"gopkg.in/yaml.v2"
)

type config struct {
	Socket   string `yaml:"socket"`
	Network  string `yaml:"network"`
	Interval int64  `yaml:"interval"`
	Hostfile string `yaml:"hostfile"`
	Domain   string `yaml:"domain"`
}

func run() error {
	var (
		cfg     config
		cfgpath = os.Args[1]
	)
	cfgdata, err := ioutil.ReadFile(cfgpath)
	if err != nil {
		return fmt.Errorf("read config: %v", err)
	}
	if err := yaml.Unmarshal(cfgdata, &cfg); err != nil {
		return fmt.Errorf("parse config: %v", err)
	}
	c, err := net.DialTimeout("unix", cfg.Socket, 2*time.Second)
	if err != nil {
		return fmt.Errorf("open socket: %v", err)
	}
	l := libvirt.New(c)
	if err := l.Connect(); err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}
	defer func() {
		log.Printf("shutting down daemon")
		if err := l.Disconnect(); err != nil {
			fmt.Printf("failed to disconnect: %v", err)
		}
	}()
	v, err := l.Version()
	if err != nil {
		return fmt.Errorf("failed to retrieve libvirt version: %v", err)
	}
	log.Printf("connected to libvirt %s", v)
	n, err := l.NetworkLookupByName(cfg.Network)
	if err != nil {
		return fmt.Errorf("failed to lookup network: %v", err)
	}
	log.Printf("found network %s", n.Name)

	// Enter watch loop
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	for {
		select {
		case <-sigs:
			return nil
		case <-time.After(time.Millisecond * time.Duration(cfg.Interval)):
		}
		leases, _, err := l.NetworkGetDhcpLeases(n, nil, -1, 0)
		if err != nil {
			log.Printf("failed to get leases: %v", err)
		}
		var buf bytes.Buffer
		for _, lease := range leases {
			if len(lease.Hostname) == 0 {
				continue
			}
			fmt.Fprintf(&buf, "%s\t%s.%s\n", lease.Ipaddr, lease.Hostname[0], cfg.Domain)
		}
		if err := ioutil.WriteFile(cfg.Hostfile, buf.Bytes(), 0644); err != nil {
			log.Printf("failed to write hostfile: %v", err)
		}
	}
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("error: %v", err)
	}
}
