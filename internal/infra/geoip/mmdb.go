package geoip

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/oschwald/geoip2-golang"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/requestmeta"
)

const (
	GeoIPDBURL    = "https://raw.githubusercontent.com/adysec/IP_database/main/geolite/GeoLite2-City.mmdb"
	GeoIPASNDBURL = "https://raw.githubusercontent.com/adysec/IP_database/main/geolite/GeoLite2-ASN.mmdb"
)

type IPLocation struct {
	Path    string
	DB      *geoip2.Reader
	ASNPath string
	ASNDB   *geoip2.Reader
}

func NewIPLocation(path string) (*IPLocation, error) {

	// 检查文件是否存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		logger.Infof("[GeoIP] Database not found, downloading from %s", GeoIPDBURL)
		// 文件不存在，下载数据库
		err := DownloadGeoIPDatabase(GeoIPDBURL, path)
		if err != nil {
			logger.Errorf("[GeoIP] Failed to download database: %v", err.Error())
			return nil, err
		}
		logger.Infof("[GeoIP] Database downloaded successfully")
	}

	db, err := geoip2.Open(path)
	if err != nil {
		return nil, err
	}

	ipLoc := &IPLocation{Path: path, DB: db}
	asnPath := filepath.Join(filepath.Dir(path), "GeoLite2-ASN.mmdb")
	ipLoc.ASNPath = asnPath
	if _, err := os.Stat(asnPath); os.IsNotExist(err) {
		logger.Infof("[GeoIP] ASN database not found, downloading from %s", GeoIPASNDBURL)
		if err := DownloadGeoIPDatabase(GeoIPASNDBURL, asnPath); err != nil {
			// ASN enrichment is optional. A transient download problem must not
			// turn logging metadata into an application startup dependency.
			logger.Errorf("[GeoIP] Failed to download ASN database; network organization will be omitted: %v", err)
			return ipLoc, nil
		}
		logger.Infof("[GeoIP] ASN database downloaded successfully")
	}
	if asnDB, err := geoip2.Open(asnPath); err != nil {
		logger.Errorf("[GeoIP] Failed to open ASN database; network organization will be omitted: %v", err)
	} else {
		ipLoc.ASNDB = asnDB
	}
	return ipLoc, nil
}

func (ipLoc *IPLocation) Close() error {
	if ipLoc == nil {
		return nil
	}
	var errs []error
	if ipLoc.DB != nil {
		errs = append(errs, ipLoc.DB.Close())
	}
	if ipLoc.ASNDB != nil {
		errs = append(errs, ipLoc.ASNDB.Close())
	}
	return errors.Join(errs...)
}

// Enrich performs at most one City and one ASN lookup for a public client IP.
// MMDB lookup errors are treated as misses so request handling remains
// independent from optional risk metadata.
func (ipLoc *IPLocation) Enrich(metadata requestmeta.Metadata) requestmeta.Metadata {
	ip := net.ParseIP(metadata.ClientIP)
	if ipLoc == nil || ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return requestmeta.Normalize(metadata)
	}
	if ipLoc.DB != nil {
		if record, err := ipLoc.DB.City(ip); err == nil && record != nil {
			metadata.IPCountryCode = record.Country.IsoCode
			metadata.IPCountry = preferredGeoName(record.Country.Names)
			if len(record.Subdivisions) > 0 {
				metadata.IPRegion = preferredGeoName(record.Subdivisions[0].Names)
			}
			metadata.IPCity = preferredGeoName(record.City.Names)
		}
	}
	if ipLoc.ASNDB != nil {
		if record, err := ipLoc.ASNDB.ASN(ip); err == nil && record != nil {
			metadata.IPASN = record.AutonomousSystemNumber
			metadata.IPASOrganization = record.AutonomousSystemOrganization
		}
	}
	return requestmeta.Normalize(metadata)
}

func preferredGeoName(names map[string]string) string {
	for _, language := range []string{"en", "zh-CN", "zh"} {
		if name := names[language]; name != "" {
			return name
		}
	}
	return ""
}

func DownloadGeoIPDatabase(url, path string) error {

	// 创建路径, 确保目录存在
	err := os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		logger.Errorf("[GeoIP] Failed to create directory: %v", err.Error())
		return err
	}

	// Write into a sibling temporary file so a timeout or truncated response
	// never leaves a corrupt database that blocks the next startup retry.
	out, err := os.CreateTemp(filepath.Dir(path), ".geoip-*.tmp")
	if err != nil {
		return err
	}
	tempPath := out.Name()
	committed := false
	defer func() {
		_ = out.Close()
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()

	// 请求远程文件
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("download GeoIP database: HTTP %d", resp.StatusCode)
	}

	// 保存文件
	const maxGeoIPDatabaseSize = int64(256 << 20)
	written, err := io.Copy(out, io.LimitReader(resp.Body, maxGeoIPDatabaseSize+1))
	if err != nil {
		return err
	}
	if written > maxGeoIPDatabaseSize {
		return fmt.Errorf("download GeoIP database: response exceeds %d bytes", maxGeoIPDatabaseSize)
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	committed = true
	return nil
}
