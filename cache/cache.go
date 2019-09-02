package cache

import (
	"ip-api-go-pkg"
	"ip-api-proxy/ipAPI"
	"time"
)

type Record struct {
	ExpirationTime 	time.Time		`json:"expirationTime"`
	Location		ip_api.Location	`json:"location"`
}

var RecordCache = map[string]Record{}

//TODO func to check cache for record
func GetLocation(query string, fields []string) (ip_api.Location,bool) {
	//Set timezone to UTC
	loc, _ := time.LoadLocation("UTC")
	//Check if record exists in cache map
	if record, found := RecordCache[query]; found {
		//Check if record has not expired
		if time.Now().In(loc).Sub(record.ExpirationTime) > 0 {
			//Remove record if expired and return false
			delete(RecordCache,query)
			return ip_api.Location{},false
		}

		//TODO return only the fields that were requested
		location := ip_api.Location{}
		//check if all fields are passed, if so just return location
		if len(fields) == len(ipAPI.AllowedAPIFields) {
			return record.Location, true
		} else {
			//Loop through fields and set selected fields
			for _, field := range fields {
				switch field {
				case "status":
					location.Status = record.Location.Status
				case "message":
					location.Message = record.Location.Message
				case "continent":
					location.Continent = record.Location.Continent
				case "continentCode":
					location.ContinentCode = record.Location.ContinentCode
				case "country":
					location.Country = record.Location.Country
				case "countryCode":
					location.CountryCode = record.Location.CountryCode
				case "region":
					location.Region = record.Location.Region
				case "regionName":
					location.RegionName = record.Location.RegionName
				case "city":
					location.City = record.Location.City
				case "district":
					location.District = record.Location.District
				case "zip":
					location.ZIP = record.Location.ZIP
				case "lat":
					location.Lat = record.Location.Lat
				case "lon":
					location.Lon = record.Location.Lon
				case "timezone":
					location.Timezone = record.Location.Timezone
				case "isp":
					location.ISP = record.Location.ISP
				case "org":
					location.Org = record.Location.Org
				case "as":
					location.AS = record.Location.AS
				case "asname":
					location.ASName = record.Location.ASName
				case "reverse":
					location.Reverse = record.Location.Reverse
				case "mobile":
					location.Mobile = record.Location.Mobile
				case "proxy":
					location.Proxy = record.Location.Proxy
				case "query":
					location.Query = record.Location.Query
				}
			}
		}
		//Return location
		return location, true
	}
	//record not found in cache return false
	return ip_api.Location{},false
}

//TODO func to add record to cache
func AddLocation(query string,location ip_api.Location, expirationDuration time.Duration) {
	//Set timezone to UTC
	loc, _ := time.LoadLocation("UTC")

	//Get expiration time
	expirationTime := time.Now().In(loc).Add(expirationDuration)

	//Create and Add record to cache
	RecordCache[query] = Record{
		ExpirationTime: expirationTime,
		Location:       location,
	}
}

//TODO func to remove records from cache after expire time
func CleanUpCache() {
	//set timezone
	loc, _ := time.LoadLocation("UTC")

	//get time.Now
	currentTime := time.Now().In(loc)

	//Loop through map and remove expired times
	for query, record := range RecordCache {
		if currentTime.Sub(record.ExpirationTime) > 0 {
			delete(RecordCache,query)
		}
	}
}

//TODO func to write cache to file

//TODO func to read cache from file