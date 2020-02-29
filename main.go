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
	Network  string `yaml:"network,default:/var/run/libvirt/libvirt-sock"`
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
	defer l.Disconnect()
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
		leases, _, err := l.NetworkGetDhcpLeases(n, nil, 0, 0)
		if err != nil {
			log.Printf("failed to get leases: %v", err)
		}
		var buf bytes.Buffer
		for _, lease := range leases {
			fmt.Fprintf(&buf, "%s.%s %s\n", lease.Hostname, cfg.Domain, lease.Ipaddr)
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
