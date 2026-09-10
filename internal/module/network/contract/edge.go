package dto

// EdgeManifestResponse is deliberately independent from SubscribeResponse:
// it is a data contract for a trusted renderer, not a client configuration.
type EdgeManifestResponse struct {
	SchemaVersion string                   `json:"schema_version"`
	Revision      string                   `json:"revision"`
	GeneratedAt   string                   `json:"generated_at"`
	Subscription  EdgeManifestSubscription `json:"subscription"`
	Proxies       []EdgeManifestProxy      `json:"proxies"`
	Notices       []string                 `json:"notices"`
}

type EdgeManifestSubscription struct {
	Name                string `json:"name"`
	State               string `json:"state"`
	ExpiresAt           string `json:"expires_at,omitempty"`
	TrafficLimit        int64  `json:"traffic_limit"`
	Upload              int64  `json:"upload"`
	Download            int64  `json:"download"`
	UpdateIntervalHours int64  `json:"update_interval_hours,omitempty"`
	WebPageURL          string `json:"web_page_url,omitempty"`
}

type EdgeManifestProxy struct {
	Name      string                 `json:"name"`
	Protocol  string                 `json:"protocol"`
	Server    string                 `json:"server"`
	Port      uint16                 `json:"port"`
	UUID      string                 `json:"uuid,omitempty"`
	Password  string                 `json:"password,omitempty"`
	Cipher    string                 `json:"cipher,omitempty"`
	Flow      string                 `json:"flow,omitempty"`
	UDP       bool                   `json:"udp"`
	TLS       *EdgeManifestTLS       `json:"tls,omitempty"`
	Transport *EdgeManifestTransport `json:"transport,omitempty"`
	Tags      []string               `json:"tags,omitempty"`
	Sort      int                    `json:"sort"`
}

type EdgeManifestTLS struct {
	Enabled     bool     `json:"enabled"`
	ServerName  string   `json:"server_name,omitempty"`
	Insecure    bool     `json:"insecure,omitempty"`
	ALPN        []string `json:"alpn,omitempty"`
	Fingerprint string   `json:"fingerprint,omitempty"`
}

type EdgeManifestTransport struct {
	Type        string `json:"type"`
	Path        string `json:"path,omitempty"`
	Host        string `json:"host,omitempty"`
	ServiceName string `json:"service_name,omitempty"`
}
