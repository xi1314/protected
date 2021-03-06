package protected

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/getlantern/golog"
	"github.com/stretchr/testify/assert"
)

const (
	testAddr = "example.com:80"
)

type testprotector struct {
	lastProtected int
}

func (p *testprotector) Protect(fileDescriptor int) error {
	p.lastProtected = fileDescriptor
	return nil
}

func TestConnectIPv4(t *testing.T) {
	doTestConnectIP(t, "8.8.8.8")
}

func TestConnectIPv6(t *testing.T) {
	dnsServer := "2001:4860:4860::8888"
	conn, err := net.Dial("udp6", fmt.Sprintf("[%v]:53", dnsServer))
	if err != nil {
		log.Debugf("Unable to dial IPv6 DNS server, assuming IPv6 not supported on this network: %v", err)
		return
	}
	conn.Close()
	doTestConnectIP(t, dnsServer)
}

func doTestConnectIP(t *testing.T, dnsServer string) {
	p := &testprotector{}
	pt := New(p.Protect, dnsServer)
	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(netw, addr string) (net.Conn, error) {
				resolved, err := pt.ResolveTCP("tcp", addr)
				if err != nil {
					return nil, err
				}
				return pt.Dial(netw, resolved.String())
			},
			ResponseHeaderTimeout: time.Second * 2,
		},
	}
	err := sendTestRequest(client, testAddr)
	if assert.NoError(t, err, "Request should have succeeded") {
		assert.NotEqual(t, 0, p.lastProtected, "Should have gotten file descriptor from protecting")
	}
}

func TestConnectHost(t *testing.T) {
	p := &testprotector{}
	pt := New(p.Protect, "8.8.8.8")
	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(netw, addr string) (net.Conn, error) {
				return pt.Dial(netw, addr)
			},
			ResponseHeaderTimeout: time.Second * 2,
		},
	}
	err := sendTestRequest(client, testAddr)
	if assert.NoError(t, err, "Request should have succeeded") {
		assert.NotEqual(t, 0, p.lastProtected, "Should have gotten file descriptor from protecting")
	}
}

func TestUDP(t *testing.T) {
	l, err := net.ListenPacket("udp", ":53243")
	if !assert.NoError(t, err) {
		return
	}
	go func() {
		b := make([]byte, 4)
		_, addr, err := l.ReadFrom(b)
		if !assert.NoError(t, err) {
			return
		}
		l.WriteTo(b, addr)
	}()

	p := &testprotector{}
	pt := New(p.Protect, "8.8.8.8")
	conn, err := pt.Dial("udp", l.LocalAddr().String())
	if !assert.NoError(t, err) {
		return
	}
	defer conn.Close()

	_, err = conn.Write([]byte("echo"))
	if !assert.NoError(t, err) {
		return
	}
	b := make([]byte, 4)
	_, err = conn.Read(b)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "echo", string(b))
	assert.NotEqual(t, 0, p.lastProtected, "Should have gotten file descriptor from protecting")
}

func sendTestRequest(client *http.Client, addr string) error {
	log := golog.LoggerFor("protected")

	req, err := http.NewRequest("GET", "http://"+addr+"/", nil)
	if err != nil {
		return fmt.Errorf("Error constructing new HTTP request: %s", err)
	}
	req.Header.Add("Connection", "keep-alive")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Could not make request to %s: %s", addr, err)
	}
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Error reading response body: %s", err)
	}
	resp.Body.Close()
	log.Debugf("Successfully processed request to %s", addr)
	return nil
}

func TestNoZone(t *testing.T) {
	assert.Equal(t, "68.105.28.11", noZone("[68.105.28.11]"))
	assert.Equal(t, "2001:4860:4860::8888", noZone("2001:4860:4860::8888"))
	assert.Equal(t, "2001:4860:4860::8888", noZone("[2001:4860:4860::8888]"))
	assert.Equal(t, "2001:4860:4860::8888", noZone("2001:4860:4860::8888%wlan0"))
	assert.Equal(t, "2001:4860:4860::8888", noZone("[2001:4860:4860::8888%wlan0]"))
}
