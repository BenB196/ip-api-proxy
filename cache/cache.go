package cache

import (
	"encoding/gob"
	"errors"
	"github.com/BenB196/ip-api-go-pkg"
	"github.com/BenB196/ip-api-proxy/promMetrics"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

type Record struct {
	ExpirationTime 	time.Time		`json:"expirationTime"`
	Location		ip_api.Location	`json:"location"`
}

var RecordCache = map[string]Record{}
var RecordCacheMutex = sync.RWMutex{}

/*
GetLocation - function for getting the location of a query from cache
query - IP/DNS entry
fields - string of comma separated values

returns
ip_api Location
error
 */
func GetLocation(query string, fields string) (ip_api.Location,bool) {
	//Set timezone to UTC
	loc, _ := time.LoadLocation("UTC")
	//Check if record exists in cache map
	RecordCacheMutex.RLock()
	if record, found := RecordCache[query]; found {
		RecordCacheMutex.RUnlock()
		//Check if record has not expired
		if time.Now().In(loc).Sub(record.ExpirationTime) > 0 {
			//Remove record if expired and return false
			promMetrics.DecreaseQueriesCachedCurrent()
			delete(RecordCache,query)
			return ip_api.Location{},false
		}

		location := ip_api.Location{}

		//Set default fields if fields string is empty
		if fields == "" {
			fields = "query,status,country,countryCode,region,regionName,city,zip,lat,lon,timezone,isp,org,as"
		}

		//check if all fields are passed, if so just return location
		if len(fields) == len(ip_api.AllowedAPIFields) {
			return record.Location, true
		} else {
			fieldSlice := strings.Split(fields,",")
			//Loop through fields and set selected fields
			for _, field := range fieldSlice {
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
	} else {
		RecordCacheMutex.RUnlock()
	}
	//record not found in cache return false
	return ip_api.Location{},false
}

/*
AddLocation - adds a query + location to cache map along with an expiration time
query - IP/DNS value
location - ip_api location
expirationDuration - duration in which the query will expire (go stale)
 */
func AddLocation(query string,location ip_api.Location, expirationDuration time.Duration) {
	//Set timezone to UTC
	loc, _ := time.LoadLocation("UTC")

	//Get expiration time
	expirationTime := time.Now().In(loc).Add(expirationDuration)

	//Create and Add record to cache
	RecordCacheMutex.Lock()
	RecordCache[query] = Record{
		ExpirationTime: expirationTime,
		Location:       location,
	}
	RecordCacheMutex.Unlock()
	promMetrics.IncrementQueriesCachedTotal()
	promMetrics.IncrementQueriesCachedCurrent()
}

/*
CleanUpCache - function which removes expired (stale) query/locations from the Cache
 */
func CleanUpCache() {
	log.Println("Starting Cache Clean Up")
	//set timezone
	loc, _ := time.LoadLocation("UTC")

	//get time.Now
	currentTime := time.Now().In(loc)

	//Loop through map and remove expired time
	for query, record := range RecordCache {
		RecordCacheMutex.Lock()
		if currentTime.Sub(record.ExpirationTime) > 0 {
			promMetrics.DecreaseQueriesCachedCurrent()
			delete(RecordCache,query)
		}
		RecordCacheMutex.Unlock()
	}
	log.Println("Finished Cache Clean Up")
}

/*
WriteCache - writes the cache to a file on disk to be read on app restarts
writeLocation - string containing the write path
 */
func WriteCache(writeLocation *string) {
	log.Println("Starting Cache Write")
	//create file name
	fileName := *writeLocation + "cache.gob"

	//create cache file
	file, err := os.Create(fileName)

	defer func() {
		if err := file.Close(); err != nil {
			panic(errors.New("error: closing file: " + fileName + " " + err.Error()))
		}
	}()

	if err != nil {
		panic(err)
	}

	//gob encoder
	e := gob.NewEncoder(file)

	//encode cache
	RecordCacheMutex.RLock()
	err = e.Encode(RecordCache)
	RecordCacheMutex.RUnlock()

	if err != nil {
		panic(err)
	}
	log.Println("Finished Cache Write")
}

/*
ReadCache - reads the cache file from disk and loads it into the Cache map
writeLocation - string containing the file path.
 */
func ReadCache(writeLocation *string) {
	//create filename
	fileName := *writeLocation + "cache.gob"

	//read file data
	cacheFile, err := os.Open(fileName)

	if err != nil {
		//If file does not exist create one
		if strings.Contains(err.Error(), "The system cannot find the file specified") || strings.Contains(err.Error(), "no such file or directory") {
			WriteCache(writeLocation)
		} else {
			panic(err)
		}
	} else {
		defer func() {
			if err := cacheFile.Close(); err != nil {
				panic(errors.New("error: closing file: " + fileName + " " + err.Error()))
			}
		}()
		//check if file size > 0
		fstat, err := cacheFile.Stat()

		if err != nil {
			panic(err)
		}

		if fstat.Size() > 0 {
			//create decode
			cacheDecoder := gob.NewDecoder(cacheFile)

			//decode cache data
			RecordCacheMutex.Lock()
			err = cacheDecoder.Decode(&RecordCache)
			RecordCacheMutex.Unlock()

			if err != nil {
				panic(err)
			}
		}
	}
}