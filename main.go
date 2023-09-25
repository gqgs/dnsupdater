package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
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
		ip, err := externalIP()
		if err != nil {
			slog.Error("failed getting external IP", "error", err)
			continue
		}
		if err = updateDNS(o, ip); err != nil {
			slog.Error("failed updating DNS", "error", err)
		}
	}

	return nil
}

type IPResponse struct {
	Query string `json:"query"`
}

func externalIP() (net.IP, error) {
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

func updateDNS(o options, ip net.IP) error {
	url := fmt.Sprintf("http://%s:%s@dynupdate.no-ip.com/nic/update?hostname=%s&myip=%s", o.username, o.password, o.dns, ip)
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
