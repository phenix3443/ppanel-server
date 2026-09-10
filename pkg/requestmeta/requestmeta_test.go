package requestmeta

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestMetadataRoundTripAndActor(t *testing.T) {
	metadata := New("203.0.113.1", "RiskClient/1.0")
	metadata.IPCountryCode = "SG"
	metadata.IPCountry = "Singapore"
	metadata.IPASN = 64500
	metadata.IPASOrganization = "Example Network"
	ctx := With(context.Background(), metadata)
	ctx = WithActor(ctx, 17)
	metadata, ok := From(ctx)
	if !ok || metadata.ClientIP != "203.0.113.1" || metadata.UserAgent != "RiskClient/1.0" || metadata.ActorID != 17 ||
		metadata.IPCountryCode != "SG" || metadata.IPCountry != "Singapore" || metadata.IPASN != 64500 || metadata.IPASOrganization != "Example Network" {
		t.Fatalf("metadata = %+v, ok = %v", metadata, ok)
	}
}

func TestBoundPreservesUTF8(t *testing.T) {
	value := strings.Repeat("客", 300)
	got := Bound(value, MaxUserAgentBytes)
	if len(got) > MaxUserAgentBytes || !utf8.ValidString(got) || !strings.HasPrefix(value, got) {
		t.Fatalf("bounded value is invalid: bytes=%d valid=%v", len(got), utf8.ValidString(got))
	}
}

func TestNormalizeBoundsDerivedMetadata(t *testing.T) {
	metadata := Normalize(Metadata{IPMetadata: IPMetadata{
		IPCountryCode:    strings.Repeat("C", MaxCountryCodeBytes+1),
		IPCountry:        strings.Repeat("C", MaxCountryBytes+1),
		IPRegion:         strings.Repeat("R", MaxRegionBytes+1),
		IPCity:           strings.Repeat("客", MaxCityBytes),
		IPASOrganization: strings.Repeat("O", MaxASOrganizationBytes+1),
	}})
	if len(metadata.IPCountryCode) > MaxCountryCodeBytes || len(metadata.IPCountry) > MaxCountryBytes ||
		len(metadata.IPRegion) > MaxRegionBytes || len(metadata.IPCity) > MaxCityBytes ||
		len(metadata.IPASOrganization) > MaxASOrganizationBytes || !utf8.ValidString(metadata.IPCity) {
		t.Fatalf("derived metadata is not bounded: %+v", metadata)
	}
}
