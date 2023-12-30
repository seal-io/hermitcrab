package registry

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/seal-io/walrus/utils/json"
	"github.com/seal-io/walrus/utils/req"
	"github.com/seal-io/walrus/utils/version"
)

var httpCli = req.HTTP().
	WithInsecureSkipVerifyEnabled().
	WithUserAgent(version.GetUserAgentWith("hermitcrab"))

type Host string

// Discover discovers the given service endpoint by the given service type.
// See https://developer.hashicorp.com/terraform/internals/remote-service-discovery.
//
// Response example:
//
//	{
//	"modules.v1": "https://modules.example.com/v1/",
//	"providers.v1": "/terraform/providers/v1/"
//	}
//

func (h Host) Discover(ctx context.Context, service string) url.URL {
	var (
		u = &url.URL{
			Scheme: "https",
			Host:   string(h),
		}
		b = map[string]string{}
	)

	err := httpCli.Request().
		GetWithContext(ctx, resolveURLString(u, "/.well-known/terraform.json")).
		BodyJSON(&b)
	if err == nil && b[service] != "" {
		return *resolveURL(u, b[service])
	}

	return *u
}

type Provider url.URL

// Provider switches the host to the provider endpoint.
func (h Host) Provider(ctx context.Context) Provider {
	switch h {
	case "registry.terraform.io":
		return Provider(url.URL{
			Scheme: "https",
			Host:   "registry.terraform.io",
			Path:   "/v1/providers/",
		})
	case "registry.opentofu.org":
		return Provider(url.URL{
			Scheme: "https",
			Host:   "registry.opentofu.org",
			Path:   "/v1/providers/",
		})
	}

	return Provider(h.Discover(ctx, "providers.v1"))
}

// GetVersions fetches the provider version list by the given parameters.
// See https://developer.hashicorp.com/terraform/internals/provider-registry-protocol#list-available-versions.
//
// Response example:
//
//	{
//	 "versions": [
//	   {
//	     "version": "2.0.0",
//	     "protocols": ["4.0", "5.1"],
//	     "platforms": [
//	       {"os": "darwin", "arch": "amd64"},
//	       {"os": "linux", "arch": "amd64"},
//	       {"os": "linux", "arch": "arm"},
//	       {"os": "windows", "arch": "amd64"}
//	     ]
//	   },
//	   {
//	     "version": "2.0.1",
//	     "protocols": ["5.2"],
//	     "platforms": [
//	       {"os": "darwin", "arch": "amd64"},
//	       {"os": "linux", "arch": "amd64"},
//	       {"os": "linux", "arch": "arm"},
//	       {"os": "windows", "arch": "amd64"}
//	     ]
//	   }
//	 ]
//	}
//
// If the given since is not zero, and the remote has not modified, the function returns nil, nil.
//

func (p Provider) GetVersions(ctx context.Context, namespace, type_ string, since ...time.Time) ([]byte, error) {
	rq := httpCli.Request()
	if len(since) != 0 && !since[0].IsZero() {
		rq = rq.WithHeader("If-Modified-Since", since[0].Format(http.TimeFormat))
	}

	r := rq.GetWithContext(ctx,
		resolveURLString((*url.URL)(&p), path.Join(namespace, type_, "versions")))

	if len(since) != 0 && !since[0].IsZero() && r.StatusCode() == http.StatusNotModified {
		return nil, nil
	}

	bs, err := r.BodyBytes()
	if err != nil {
		return nil, err
	}

	if json.Get(bs, "versions").IsArray() {
		return bs, nil
	}

	return []byte(`{"versions":[]}`), nil
}

// GetPlatform fetches the provider versioned platform information by the given parameters.
// See https://developer.hashicorp.com/terraform/internals/provider-registry-protocol#find-a-provider-package.
//
// Response example:
//
//	{
//	 "protocols": ["4.0", "5.1"],
//	 "os": "linux",
//	 "arch": "amd64",
//	 "filename": "terraform-provider-random_2.0.0_linux_amd64.zip",
//	 "download_url": "https://releases.hashicorp.com/terraform-provider-random/2.0.0/terraform-provider-random_2.0.0_linux_amd64.zip",
//	 "shasums_url": "https://releases.hashicorp.com/terraform-provider-random/2.0.0/terraform-provider-random_2.0.0_SHA256SUMS",
//	 "shasums_signature_url": "https://releases.hashicorp.com/terraform-provider-random/2.0.0/terraform-provider-random_2.0.0_SHA256SUMS.sig",
//	 "shasum": "5f9c7aa76b7c34d722fc9123208e26b22d60440cb47150dd04733b9b94f4541a",
//	 "signing_keys": {
//	   "gpg_public_keys": [
//	     {
//	       "key_id": "51852D87348FFC4C",
//	       "ascii_armor": "-----BEGIN PGP PUBLIC KEY BLOCK-----\nVersion: GnuPG v1\n\nmQENBFMORM0BCADBRyKO1MhCirazOSVwcfTr1xUxjPvfxD3hjUwHtjsOy/bT6p9f\nW2mRPfwnq2JB5As+paL3UGDsSRDnK9KAxQb0NNF4+eVhr/EJ18s3wwXXDMjpIifq\nfIm2WyH3G+aRLTLPIpscUNKDyxFOUbsmgXAmJ46Re1fn8uKxKRHbfa39aeuEYWFA\n3drdL1WoUngvED7f+RnKBK2G6ZEpO+LDovQk19xGjiMTtPJrjMjZJ3QXqPvx5wca\nKSZLr4lMTuoTI/ZXyZy5bD4tShiZz6KcyX27cD70q2iRcEZ0poLKHyEIDAi3TM5k\nSwbbWBFd5RNPOR0qzrb/0p9ksKK48IIfH2FvABEBAAG0K0hhc2hpQ29ycCBTZWN1\ncml0eSA8c2VjdXJpdHlAaGFzaGljb3JwLmNvbT6JATgEEwECACIFAlMORM0CGwMG\nCwkIBwMCBhUIAgkKCwQWAgMBAh4BAheAAAoJEFGFLYc0j/xMyWIIAIPhcVqiQ59n\nJc07gjUX0SWBJAxEG1lKxfzS4Xp+57h2xxTpdotGQ1fZwsihaIqow337YHQI3q0i\nSqV534Ms+j/tU7X8sq11xFJIeEVG8PASRCwmryUwghFKPlHETQ8jJ+Y8+1asRydi\npsP3B/5Mjhqv/uOK+Vy3zAyIpyDOMtIpOVfjSpCplVRdtSTFWBu9Em7j5I2HMn1w\nsJZnJgXKpybpibGiiTtmnFLOwibmprSu04rsnP4ncdC2XRD4wIjoyA+4PKgX3sCO\nklEzKryWYBmLkJOMDdo52LttP3279s7XrkLEE7ia0fXa2c12EQ0f0DQ1tGUvyVEW\nWmJVccm5bq25AQ0EUw5EzQEIANaPUY04/g7AmYkOMjaCZ6iTp9hB5Rsj/4ee/ln9\nwArzRO9+3eejLWh53FoN1rO+su7tiXJA5YAzVy6tuolrqjM8DBztPxdLBbEi4V+j\n2tK0dATdBQBHEh3OJApO2UBtcjaZBT31zrG9K55D+CrcgIVEHAKY8Cb4kLBkb5wM\nskn+DrASKU0BNIV1qRsxfiUdQHZfSqtp004nrql1lbFMLFEuiY8FZrkkQ9qduixo\nmTT6f34/oiY+Jam3zCK7RDN/OjuWheIPGj/Qbx9JuNiwgX6yRj7OE1tjUx6d8g9y\n0H1fmLJbb3WZZbuuGFnK6qrE3bGeY8+AWaJAZ37wpWh1p0cAEQEAAYkBHwQYAQIA\nCQUCUw5EzQIbDAAKCRBRhS2HNI/8TJntCAClU7TOO/X053eKF1jqNW4A1qpxctVc\nz8eTcY8Om5O4f6a/rfxfNFKn9Qyja/OG1xWNobETy7MiMXYjaa8uUx5iFy6kMVaP\n0BXJ59NLZjMARGw6lVTYDTIvzqqqwLxgliSDfSnqUhubGwvykANPO+93BBx89MRG\nunNoYGXtPlhNFrAsB1VR8+EyKLv2HQtGCPSFBhrjuzH3gxGibNDDdFQLxxuJWepJ\nEK1UbTS4ms0NgZ2Uknqn1WRU1Ki7rE4sTy68iZtWpKQXZEJa0IGnuI2sSINGcXCJ\noEIgXTMyCILo34Fa/C6VCm2WBgz9zZO8/rHIiQm1J5zqz0DrDwKBUM9C\n=LYpS\n-----END PGP PUBLIC KEY BLOCK-----",
//	       "trust_signature": "",
//	       "source": "HashiCorp",
//	       "source_url": "https://www.hashicorp.com/security.html"
//	     }
//	   ]
//	 }
//	}
//
// If the given since is not zero, and the remote has not modified, the function returns nil, nil.
//
//nolint:lll
func (p Provider) GetPlatform(
	ctx context.Context,
	namespace, type_, version, os, arch string,
	since ...time.Time,
) ([]byte, error) {
	rq := httpCli.Request()
	if len(since) != 0 && !since[0].IsZero() {
		rq = rq.WithHeader("If-Modified-Since", since[0].Format(http.TimeFormat))
	}

	r := rq.GetWithContext(ctx,
		resolveURLString((*url.URL)(&p), path.Join(namespace, type_, version, "download", os, arch)),
	)

	if len(since) != 0 && !since[0].IsZero() && r.StatusCode() == http.StatusNotModified {
		return nil, nil
	}

	bs, err := r.BodyBytes()
	if err != nil {
		return nil, err
	}

	if json.Get(bs, "@this").IsObject() {
		return bs, nil
	}

	return []byte(`{}`), nil
}

type Module url.URL

// Module switches the host to the module endpoint.
func (h Host) Module(ctx context.Context) Module {
	return Module(h.Discover(ctx, "modules.v1"))
}

// GetVersions fetches the module version list by the given parameters.
// See https://developer.hashicorp.com/terraform/internals/module-registry-protocol#list-available-versions-for-a-specific-module.
// Response example:
//
//	{
//	  "modules": [
//	     {
//	        "versions": [
//	           {"version": "1.0.0"},
//	           {"version": "1.1.0"},
//	           {"version": "2.0.0"}
//	        ]
//	     }
//	  ]
//	}
//
// If the given since is not zero, and the remote has not modified, the function returns nil, nil.
//
//nolint:lll
func (m Module) GetVersions(ctx context.Context, namespace, name, system string, since ...time.Time) ([]byte, error) {
	rq := httpCli.Request()
	if len(since) != 0 && !since[0].IsZero() {
		rq = rq.WithHeader("If-Modified-Since", since[0].Format(http.TimeFormat))
	}

	r := rq.GetWithContext(ctx,
		resolveURLString((*url.URL)(&m), path.Join(namespace, name, system, "versions")))

	if len(since) != 0 && !since[0].IsZero() && r.StatusCode() == http.StatusNotModified {
		return nil, nil
	}

	bs, err := r.BodyBytes()
	if err != nil {
		return nil, err
	}

	if json.Get(bs, "modules").IsArray() {
		return bs, nil
	}

	return []byte(`{"modules":[]}`), nil
}

// GetVersion fetches the module versioned information by the given parameters.
// See https://developer.hashicorp.com/terraform/internals/module-registry-protocol#download-source-code-for-a-specific-module-version.
// Response example:
//
//	{
//	 "download_url": "https://api.github.com/repos/hashicorp/terraform-aws-consul/tarball/v0.0.1//*?archive=tar.gz"
//	}
//
// If the given since is not zero, and the remote has not modified, the function returns nil, nil.
//
//nolint:lll
func (m Module) GetVersion(
	ctx context.Context,
	namespace, name, system, version string,
	since ...time.Time,
) ([]byte, error) {
	rq := httpCli.Request()
	if len(since) != 0 && !since[0].IsZero() {
		rq = rq.WithHeader("If-Modified-Since", since[0].Format(http.TimeFormat))
	}

	r := rq.GetWithContext(ctx,
		resolveURLString((*url.URL)(&m), path.Join(namespace, name, system, version, "download")),
	)

	if len(since) != 0 && !since[0].IsZero() && r.StatusCode() == http.StatusNotModified {
		return nil, nil
	}

	downloadURL := r.Header("X-Terraform-Get")
	if downloadURL != "" {
		return []byte(`{"download_url":"` + downloadURL + `"}`), nil
	}

	return []byte(`{}`), nil
}

func resolveURL(u *url.URL, p string) *url.URL {
	return u.ResolveReference(&url.URL{Path: p})
}

func resolveURLString(u *url.URL, p string) string {
	return resolveURL(u, p).String()
}
