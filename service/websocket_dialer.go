package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/gorilla/websocket"
	"golang.org/x/net/proxy"
)

func NewWebSocketDialer(proxyURL string) (*websocket.Dialer, error) {
	dialer := &websocket.Dialer{
		Proxy:             http.ProxyFromEnvironment,
		EnableCompression: true,
		HandshakeTimeout:  45 * time.Second,
	}

	if common.RelayTimeout > 0 {
		dialer.HandshakeTimeout = time.Duration(common.RelayTimeout) * time.Second
	}
	if common.TLSInsecureSkipVerify {
		dialer.TLSClientConfig = common.InsecureTLSConfig
	}

	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return dialer, nil
	}

	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}

	switch parsedURL.Scheme {
	case "http", "https":
		dialer.Proxy = http.ProxyURL(parsedURL)
		return dialer, nil
	case "socks5", "socks5h":
		var auth *proxy.Auth
		if parsedURL.User != nil {
			auth = &proxy.Auth{
				User: parsedURL.User.Username(),
			}
			if password, ok := parsedURL.User.Password(); ok {
				auth.Password = password
			}
		}

		socksDialer, err := proxy.SOCKS5("tcp", parsedURL.Host, auth, proxy.Direct)
		if err != nil {
			return nil, err
		}

		dialer.Proxy = nil
		dialer.NetDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			type dialResult struct {
				conn net.Conn
				err  error
			}

			resultCh := make(chan dialResult, 1)
			go func() {
				conn, dialErr := socksDialer.Dial(network, addr)
				resultCh <- dialResult{conn: conn, err: dialErr}
			}()

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case result := <-resultCh:
				return result.conn, result.err
			}
		}
		return dialer, nil
	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s, must be http, https, socks5 or socks5h", parsedURL.Scheme)
	}
}
