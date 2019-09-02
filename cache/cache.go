package cache

import (
	"ip-api-go-pkg"
	"time"
)

type Record struct {
	ExpirationTime 	time.Time		`json:"expirationTime"`
	Location		ip_api.Location	`json:"location"`
}

var RecordCache = map[string]Record{}

//TODO func to check cache for record
func GetLocation(query string) (ip_api.Location,bool) {
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
		//Return location
		return record.Location, true
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