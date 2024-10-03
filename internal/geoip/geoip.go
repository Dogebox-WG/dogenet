package geoip

import (
	"encoding/csv"
	"log"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/philpearl/intern"
)

type IPRecord struct {
	Start     uint32 // Starting IP of the range
	End       uint32 // Ending IP of the range
	Latitude  string
	Longitude string
	Country   string
	City      string
}

type GeoIPDatabase struct {
	Records []IPRecord
	index   []uint32
	intern  *intern.Intern
}

func NewGeoIPDatabase(filename string) (*GeoIPDatabase, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	r := csv.NewReader(file)
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}

	db := &GeoIPDatabase{
		Records: make([]IPRecord, 0, len(records)),
		index:   make([]uint32, 0, len(records)),
		intern:  intern.New(256),
	}

	for i, record := range records {
		country := db.intern.Deduplicate(strings.TrimSpace(record[2]))
		city := db.intern.Deduplicate(substr(strings.TrimSpace(record[5]), 30)) // profile limit
		start, e1 := strconv.ParseInt(record[0], 10, 0)
		end, e2 := strconv.ParseInt(record[1], 10, 0)
		if e1 == nil && e2 == nil {
			db.index = append(db.index, uint32(start))
			db.Records = append(db.Records, IPRecord{
				Start:     uint32(start),
				End:       uint32(end),
				Latitude:  record[7],
				Longitude: record[8],
				Country:   country,
				City:      city,
			})
		} else {
			log.Printf("invalid row: %v: %v", i, record)
		}
	}

	return db, nil
}

func substr(s string, n int) string {
	if len(s) > n {
		return s[0:n]
	}
	return s
}

func (db *GeoIPDatabase) FindLocation(ip net.IP) (string, string, string, string) {
	ipLong := ipToUint32(ip.To4())
	if ipLong != 0 {
		// 0 <= pos <= len(index)
		pos := SearchUInt32(db.index, ipLong)
		if pos > 0 {
			// usually our search result (insertion-point) is immediately
			// after the record we're looking for.
			rec := db.Records[pos-1]
			if ipLong >= rec.Start && ipLong <= rec.End {
				return rec.Latitude, rec.Longitude, rec.Country, rec.City
			}
		}
		if pos < len(db.Records) {
			// however, if our address exactly equals the Start address,
			// we'll get an exact record index.
			rec := db.Records[pos]
			if ipLong >= rec.Start && ipLong <= rec.End {
				return rec.Latitude, rec.Longitude, rec.Country, rec.City
			}
		}
	}
	return "0.0", "0.0", "", ""
}

func SearchUInt32(a []uint32, x uint32) int {
	return sort.Search(len(a), func(i int) bool { return a[i] >= x })
}

func ipToUint32(ip net.IP) uint32 {
	var ipLong uint32
	for _, part := range ip {
		ipLong = (ipLong << 8) | uint32(part)
	}
	return ipLong
}
