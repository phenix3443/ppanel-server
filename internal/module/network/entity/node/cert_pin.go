package node

import "strings"

// NormalizeCertPinSHA256 canonicalizes a node-reported TLS certificate
// fingerprint to lowercase hex. It returns "" when the value is not a SHA256
// hex digest, so callers can treat invalid input as "not reported".
func NormalizeCertPinSHA256(raw string) string {
	pin := strings.ToLower(strings.TrimSpace(raw))
	if len(pin) != 64 {
		return ""
	}
	for _, r := range pin {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return ""
		}
	}
	return pin
}

// ApplyReportedCertPin records the certificate fingerprint a node reported for
// its protocol type. Only self-signed entries (cert_mode=self) accept the
// report; the node computes the fingerprint solely for certificates it
// generated itself, so a pin against any other cert_mode is stale by
// definition. It returns true when the stored protocols changed.
func (m *Server) ApplyReportedCertPin(protocolType, fingerprint string) (bool, error) {
	pin := NormalizeCertPinSHA256(fingerprint)
	if pin == "" {
		return false, nil
	}
	protocols, err := m.UnmarshalProtocols()
	if err != nil {
		return false, err
	}
	target := normalizeProtocolType(protocolType)
	changed := false
	for i := range protocols {
		if normalizeProtocolType(protocols[i].Type) != target {
			continue
		}
		if normalizeNone(protocols[i].CertMode) != "self" || protocols[i].CertPinSHA256 == pin {
			continue
		}
		protocols[i].CertPinSHA256 = pin
		changed = true
	}
	if !changed {
		return false, nil
	}
	return true, m.MarshalProtocols(protocols)
}
