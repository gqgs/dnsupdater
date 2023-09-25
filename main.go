package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

//go:generate go run github.com/gqgs/argsgen

type options struct {
	interval string `arg:"updater internal"`
	username string `arg:"NoIP username,required"`
	password string `arg:"NoIP password,required"`
	dns      string `arg:"NoIP DNS,required"`
}

func main() {
	if err := process(); err != nil {
		log.Fatal(err)
	}
}

func process() error {
	o := options{
		interval: "1h",
	}
	o.MustParse()

	internal, err := time.ParseDuration(o.interval)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(internal)
	defer ticker.Stop()

	for ; true; <-ticker.C {
		var ips []net.IP
		ipv4, err := externalIPv4()
		if err != nil {
			slog.Error("failed getting external IPv4", "error", err)
		} else {
			ips = append(ips, ipv4)
		}
		ipv6, err := externalIPv6()
		if err != nil {
			slog.Error("failed getting external IPv6", "error", err)
		} else {
			ips = append(ips, ipv6)
		}

		if len(ips) == 0 {
			slog.Error("not enough ips found")
			continue
		}

		if err = updateDNS(o, ips...); err != nil {
			slog.Error("failed updating DNS", "error", err)
		}
	}

	return nil
}

type IPResponse struct {
	Query string `json:"query"`
}

func externalIPv4() (net.IP, error) {
	resp, err := http.Get("http://ip-api.com/json/?fields=query")
	if err != nil {
		return net.IP{}, err
	}
	defer resp.Body.Close()

	parsed := new(IPResponse)
	if err = json.NewDecoder(resp.Body).Decode(parsed); err != nil {
		return net.IP{}, err
	}

	return net.ParseIP(parsed.Query), nil
}

func externalIPv6() (net.IP, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return net.IP{}, err
	}
	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			return net.IP{}, err
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip.IsLoopback() || ip.IsPrivate() {
				continue
			}

			return ip, nil
		}
	}
	return net.IP{}, errors.New("not found")
}

func updateDNS(o options, ips ...net.IP) error {
	var ipList []string
	for _, ip := range ips {
		ipList = append(ipList, ip.String())
	}

	url := fmt.Sprintf("http://%s:%s@dynupdate.no-ip.com/nic/update?hostname=%s&myip=%s", o.username, o.password, o.dns, strings.Join(ipList, ","))
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	req.SetBasicAuth(o.username, o.password)
	req.Header.Add("User-Agent", "Linux-DUC/2.1.9")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	slog.Info("dns updated", "response", string(data))

	return nil
}
