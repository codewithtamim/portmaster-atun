package geoip

import (
	"errors"
	"net"
)

// GetLocation returns Location data of an IP address.
func GetLocation(ip net.IP) (*Location, error) {
	db := getReader(ip)
	if db == nil {
		return nil, errors.New("geoip database not available")
	}
	record := &Location{}
	if err := db.Lookup(ip, record); err != nil {
		return nil, err
	}

	record.AddCountryInfo()
	return record, nil
}

// IsInitialized returns whether the geoip database has been initialized.
func IsInitialized(v6, wait bool) bool {
	return worker.GetReader(v6, wait) != nil
}
