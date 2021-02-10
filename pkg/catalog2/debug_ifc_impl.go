package catalog2

import (
	"time"

	"github.com/openservicemesh/osm/pkg/certificate"
	"github.com/openservicemesh/osm/pkg/envoy"
)

// ListExpectedProxies lists the Envoy proxies yet to connect and the time their XDS certificate was issued.
func (c *Catalog) ListExpectedProxies() map[certificate.CommonName]time.Time {
	return c.OriginalCatalog.ListExpectedProxies()
}

// ListConnectedProxies lists the Envoy proxies already connected and the time they first connected.
func (c *Catalog) ListConnectedProxies() map[certificate.CommonName]*envoy.Proxy {
	return c.OriginalCatalog.ListConnectedProxies()
}

// ListDisconnectedProxies lists the Envoy proxies disconnected and the time last seen.
func (c *Catalog) ListDisconnectedProxies() map[certificate.CommonName]time.Time {
	return c.OriginalCatalog.ListDisconnectedProxies()
}
