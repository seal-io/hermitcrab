package download

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

func NewHttpClient(opts ...HttpClientOption) *http.Client {
	hc := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	for i := range opts {
		if opts[i] == nil {
			continue
		}

		hc = opts[i](hc)
	}

	return hc
}

type HttpClientOption func(*http.Client) *http.Client

func WithTimeout(timeout time.Duration) HttpClientOption {
	if timeout == 0 {
		return nil
	}

	return func(cli *http.Client) *http.Client {
		cli.Timeout = timeout
		return cli
	}
}

func WithUserAgent(userAgent string) HttpClientOption {
	if userAgent == "" {
		return nil
	}

	return func(cli *http.Client) *http.Client {
		cli.Transport = &_CustomTransport{
			Base: cli.Transport,
			Custom: func(r *http.Request) {
				r.Header.Set("User-Agent", userAgent)
			},
		}

		return cli
	}
}

func WithInsecureSkipVerify() HttpClientOption {
	return func(cli *http.Client) *http.Client {
		for tr := cli.Transport; tr != nil; {
			switch v := tr.(type) {
			case *_CustomTransport:
				tr = v.Base
				continue
			case *http.Transport:
				if v.TLSClientConfig == nil {
					v.TLSClientConfig = &tls.Config{
						MinVersion: tls.VersionTLS12,
					}
				}
				v.TLSClientConfig.InsecureSkipVerify = true
			}

			break
		}

		return cli
	}
}

type _CustomTransport struct {
	Base   http.RoundTripper
	Custom func(*http.Request)
}

func (t *_CustomTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	t.Custom(r2)

	return t.Base.RoundTrip(r2)
}
