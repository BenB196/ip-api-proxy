package main

import (
	"encoding/json"
	"flag"
	"github.com/BenB196/ip-api-go-pkg"
	"github.com/BenB196/ip-api-proxy/cache"
	"github.com/BenB196/ip-api-proxy/config"
	"github.com/BenB196/ip-api-proxy/promMetrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

//Init config globally
var LoadedConfig = config.Config{}

var IPDNSRegexp = regexp.MustCompile(`(((25[0-5]|(2[0-4]|1?[0-9])?[0-9])\.){3}(25[0-5]|(2[0-4]|1?[0-9])?[0-9])|((([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])(\.)?$)|(([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:)|fe80:(:[0-9a-fA-F]{0,4}){0,4}%[0-9a-zA-Z]+|::(ffff(:0{1,4})?:)?((25[0-5]|(2[0-4]|1?[0-9])?[0-9])\.){3}(25[0-5]|(2[0-4]|1?[0-9])?[0-9])|([0-9a-fA-F]{1,4}:){1,4}:((25[0-5]|(2[0-4]|1?[0-9])?[0-9])\.){3}(25[0-5]|(2[0-4]|1?[0-9])?[0-9])))`)

func main()  {
	var err error

	//get config location flag
	var configLocation string
	flag.StringVar(&configLocation,"config","","Configuration file location. Defaults to working directory.")

	//Parse flags
	flag.Parse()

	//Read config
	LoadedConfig, err = config.ReadConfig(configLocation)

	if err != nil {
		panic(err)
	}

	//handle single requests
	http.HandleFunc("/json/",ipAPIJson)

	//handle batch requests
	http.HandleFunc("/batch",ipAPIBatch)

	if LoadedConfig.Prometheus.Enabled {
		//Start prometheus metrics end point
		http.Handle("/metrics",promhttp.Handler())
	}

	//404 everything else
	http.HandleFunc("/",ipAIPProxy)

	//Write cache if persist is true
	if LoadedConfig.Cache.Persist {
		//read cache file on startup
		cache.ReadCache(&LoadedConfig.Cache.WriteLocation)

		var writeCacheWg sync.WaitGroup
		writeCacheDuration, _ := time.ParseDuration(LoadedConfig.Cache.WriteInterval)
		writeCacheTimeTicker := time.NewTicker(writeCacheDuration)
		writeCacheWg.Add(1)

		go func() {
			for {
				select {
				case <-writeCacheTimeTicker.C:
					cache.WriteCache(&LoadedConfig.Cache.WriteLocation)
				}
				defer writeCacheWg.Done()
			}
		}()
	}

	//Listen on port
	log.Println("Starting server on port " + strconv.Itoa(LoadedConfig.Port) + "...")
	if err := http.ListenAndServe(":" + strconv.Itoa(LoadedConfig.Port),nil); err != nil {
		log.Println(err)
	}
}

func ipAPIJson(w http.ResponseWriter, r *http.Request) {
	//increment requests processed
	promMetrics.IncrementRequestsProcessed()
	promMetrics.IncrementSingleRequestsProcessed()
	//increment queries processed (single query so can increment here)
	promMetrics.IncrementSingleQueriesProcessed()
	promMetrics.IncrementQueriesProcessed()

	//set content type
	w.Header().Set("Content-Type","application/json")

	//init location variable
	location := ip_api.Location{}

	//init error
	var err error

	if r.Method == "GET" {
		//check to make sure that there are only 2 or less / in URL
		if strings.Count(r.URL.Path,"/") > 2 {
			location.Status = "fail"
			location.Message = "expected one (1) or (2) \"/\" but got more."
			log.Println("Failed single request: 400 expected one (1) or (2) \"/\" but got more.")
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedRequests()
			promMetrics.IncrementFailedSingleRequests()
			jsonLocation, _ := json.Marshal(&location)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(jsonLocation)
			return
		}

		//get fields values
		fields, ok := r.URL.Query()["fields"]
		if !ok && len(fields) > 0 {
			location.Status = "fail"
			location.Message = "invalid fields provided"
			log.Println("Failed single request: invalid fields provided")
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedRequests()
			promMetrics.IncrementFailedSingleRequests()
			jsonLocation, _ := json.Marshal(&location)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(jsonLocation)
			return
		}

		//get lang value
		lang, ok := r.URL.Query()["lang"]
		if !ok && len(lang) > 0 {
			location.Status = "fail"
			location.Message = "invalid lang provided"
			log.Println("Failed single request: invalid lang provided")
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedRequests()
			promMetrics.IncrementFailedSingleRequests()
			jsonLocation, _ := json.Marshal(&location)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(jsonLocation)
			return
		}

		//get ecs value
		ecs, ok := r.URL.Query()["ecs"]
		var ecsBool = false
		if len(ecs) > 0 {
			ecsBool, _ = strconv.ParseBool(ecs[0])
		}

		//validate fields
		var validatedFields string
		if len(fields) > 0 {
			validatedFields, err = ip_api.ValidateFields(fields[0])

			if err != nil {
				location.Status = "fail"
				location.Message = err.Error()
				log.Println("Failed single request: " + err.Error())
				promMetrics.IncrementHandlerRequests("400")
				promMetrics.IncrementFailedRequests()
				promMetrics.IncrementFailedSingleRequests()
				jsonLocation, _ := json.Marshal(&location)
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write(jsonLocation)
				return
			}
		}

		//validate lang
		var validatedLang string
		if len(lang) > 0 {
			validatedLang, err = ip_api.ValidateLang(lang[0])

			if err != nil {
				location.Status = "fail"
				location.Message = err.Error()
				log.Println("Failed single request: " + err.Error())
				promMetrics.IncrementHandlerRequests("400")
				promMetrics.IncrementFailedRequests()
				promMetrics.IncrementFailedSingleRequests()
				jsonLocation, _ := json.Marshal(&location)
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write(jsonLocation)
				return
			}
		}

		//get key
		keys, ok := r.URL.Query()["key"]
		key := LoadedConfig.APIKey
		//overwrite config api if passed through url
		if len(keys) > 0 {
			key = keys[0]
		}

		//Get ip address
		ip := IPDNSRegexp.FindString(r.URL.Path)

		if ip == "" {
			location.Status = "fail"
			location.Message = "request is blank"
			log.Println("Failed single request: request is blank")
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedRequests()
			promMetrics.IncrementFailedSingleRequests()
			jsonLocation, _ := json.Marshal(&location)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(jsonLocation)
			return
		}

		//Check cache for ip
		location, found, err := cache.GetLocation(ip + validatedLang,validatedFields)

		if err != nil {
			panic(err)
		}

		//If ip found in cache return cached value
		if found {
			if LoadedConfig.Debugging {
				log.Println("Found: " + ip + " in cache.")
			}
			promMetrics.IncrementHandlerRequests("200")
			promMetrics.IncrementCacheHits()
			promMetrics.IncrementSuccessfulQueries()
			promMetrics.IncrementSuccessfulSingeQueries()
			if !ecsBool {
				jsonLocation, _ := json.Marshal(&location)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(jsonLocation)
			} else {
				ecsLocation := EcsLocation{
					Status:        location.Status,
					Message:       location.Message,
					Continent:     location.Continent,
					ContinentCode: location.ContinentCode,
					Country:       location.Country,
					CountryCode:   location.CountryCode,
					Region:        location.Region,
					RegionName:    location.RegionName,
					City:          location.City,
					District:      location.District,
					ZIP:           location.ZIP,
					Lat:           location.Lat,
					Lon:           location.Lon,
					Timezone:      location.Timezone,
					Currency:      location.Currency,
					ISP:           location.ISP,
					Org:           location.Org,
					AS:            location.AS,
					ASName:        location.ASName,
					Reverse:       location.Reverse,
					Mobile:        location.Mobile,
					Proxy:         location.Proxy,
					Hosting:       location.Hosting,
					Query:         location.Query,
				}
				jsonLocation, _ := json.Marshal(&ecsLocation)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(jsonLocation)
			}
			return
		}

		//Build query
		query := ip_api.Query{
			Queries:[]ip_api.QueryIP{
				{Query:ip},
			},
			Fields:strings.Join(ip_api.AllowedAPIFields,","), //Execute query to IP API for all fields, handle field selection later
			Lang:validatedLang,
		}

		//execute query
		promMetrics.IncrementRequestsForwarded()
		promMetrics.IncrementQueriesForwarded()
		var newLocation *ip_api.Location
		newLocation, err = ip_api.SingleQuery(query,key,"",LoadedConfig.Debugging)

		if err != nil {
			location.Status = "fail"
			location.Message = err.Error()
			log.Println("Failed single request: " + err.Error())
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedRequests()
			promMetrics.IncrementFailedSingleRequests()
			jsonLocation, _ := json.Marshal(&location)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(jsonLocation)
			return
		}

		//Add to cache if successful request
		if newLocation.Status == "success" {
			if LoadedConfig.Debugging {
				log.Println("Added: " + ip + validatedLang + " to cache.")
			}
			promMetrics.IncrementHandlerRequests("200")
			_, err = cache.AddLocation(ip + validatedLang,*newLocation,*LoadedConfig.Cache.SuccessAgeDuration)
			if err != nil {
				log.Println(err)
			}
			//Re-get request with specified fields
			newLocation, _, err = cache.GetLocation(ip + validatedLang,validatedFields)
			if err != nil {
				log.Println(err)
			}
			promMetrics.IncrementSuccessfulQueries()
			promMetrics.IncrementSuccessfulSingeQueries()
		}

		//if request failed, increment 400 and fail request counter
		if newLocation.Status == "fail" {
			log.Println("Failed single query: " + ip)
			promMetrics.IncrementHandlerRequests("400")
			_, err = cache.AddLocation(ip + validatedLang,*newLocation,*LoadedConfig.Cache.FailedAgeDuration)
			if err != nil {
				log.Println(err)
			}
			//Re-get request with specified fields
			newLocation, _, err = cache.GetLocation(ip + validatedLang,validatedFields)
			if err != nil {
				log.Println(err)
			}
			promMetrics.IncrementFailedQueries()
			promMetrics.IncrementFailedSingleQueries()

		}

		//return query
		if !ecsBool {
			jsonLocation, _ := json.Marshal(&newLocation)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonLocation)
		} else {
			ecsLocation := EcsLocation{
				Status:        newLocation.Status,
				Message:       newLocation.Message,
				Continent:     newLocation.Continent,
				ContinentCode: newLocation.ContinentCode,
				Country:       newLocation.Country,
				CountryCode:   newLocation.CountryCode,
				Region:        newLocation.Region,
				RegionName:    newLocation.RegionName,
				City:          newLocation.City,
				District:      newLocation.District,
				ZIP:           newLocation.ZIP,
				Lat:           newLocation.Lat,
				Lon:           newLocation.Lon,
				Timezone:      newLocation.Timezone,
				Currency:      newLocation.Currency,
				ISP:           newLocation.ISP,
				Org:           newLocation.Org,
				AS:            newLocation.AS,
				ASName:        newLocation.ASName,
				Reverse:       newLocation.Reverse,
				Mobile:        newLocation.Mobile,
				Proxy:         newLocation.Proxy,
				Hosting:       newLocation.Hosting,
				Query:         newLocation.Query,
			}
			jsonLocation, _ := json.Marshal(&ecsLocation)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonLocation)
		}
		return
	} else {
		if r.URL.Path != "/json/" && r.URL.Path != "/batch" && r.URL.Path != "/metrics" {
			location.Status = "fail"
			location.Message = "/json/ endpoint only supports GET requests."
			log.Println("Failed single request: /json/ endpoint only supports GET requests.")
			promMetrics.IncrementHandlerRequests("404")
			jsonLocation, _ := json.Marshal(&location)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write(jsonLocation)
			return
		}
	}
}

func ipAPIBatch(w http.ResponseWriter, r *http.Request) {
	//increment requests processed
	promMetrics.IncrementRequestsProcessed()
	promMetrics.IncrementBatchRequestsProcessed()

	//set content type
	w.Header().Set("Content-Type","application/json")

	//init location variable
	location := ip_api.Location{}

	//init error
	var err error

	if r.Method == "POST" {
		//check to make sure that there are only 1 or less / in URL
		if strings.Count(r.URL.Path,"/") > 1 {
			location.Status = "fail"
			location.Message = "expected one (1) \"/\" but got more."
			log.Println("Failed batch request: expected one (1) \"/\" but got more.")
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedRequests()
			promMetrics.IncrementFailedBatchRequests()
			jsonLocation, _ := json.Marshal(&location)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(jsonLocation)
			return
		}

		//get fields values
		fields, ok := r.URL.Query()["fields"]
		if !ok && len(fields) > 0 {
			location.Status = "fail"
			location.Message = "invalid fields provided"
			log.Println("Failed batch request: invalid fields provided")
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedRequests()
			promMetrics.IncrementFailedBatchRequests()
			jsonLocation, _ := json.Marshal(&location)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(jsonLocation)
			return
		}

		//get lang value
		lang, ok := r.URL.Query()["lang"]
		if !ok && len(lang) > 0 {
			location.Status = "fail"
			location.Message = "invalid lang provided"
			log.Println("Failed batch request: invalid lang provided")
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedRequests()
			promMetrics.IncrementFailedBatchRequests()
			jsonLocation, _ := json.Marshal(&location)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(jsonLocation)
			return
		}

		//get ecs value
		ecs, ok := r.URL.Query()["ecs"]
		var ecsBool = false
		if len(ecs) > 0 {
			ecsBool, _ = strconv.ParseBool(ecs[0])
		}

		//validate fields
		var validatedFields string
		if len(fields) > 0 {
			validatedFields, err = ip_api.ValidateFields(fields[0])

			if err != nil {
				location.Status = "fail"
				location.Message = err.Error()
				log.Println("Failed batch request: " + err.Error())
				promMetrics.IncrementHandlerRequests("400")
				promMetrics.IncrementFailedRequests()
				promMetrics.IncrementFailedBatchRequests()
				jsonLocation, _ := json.Marshal(&location)
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write(jsonLocation)
				return
			}
		}

		//validate lang
		var validatedLang string
		if len(lang) > 0 {
			validatedLang, err = ip_api.ValidateLang(lang[0])

			if err != nil {
				location.Status = "fail"
				location.Message = err.Error()
				log.Println("Failed batch request: " + err.Error())
				promMetrics.IncrementHandlerRequests("400")
				promMetrics.IncrementFailedRequests()
				promMetrics.IncrementFailedBatchRequests()
				jsonLocation, _ := json.Marshal(&location)
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write(jsonLocation)
				return
			}
		}

		//get key
		keys, ok := r.URL.Query()["key"]
		key := LoadedConfig.APIKey
		//overwrite config api if passed through url
		if len(keys) > 0 {
			key = keys[0]
		}

		//Read body data
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			location.Status = "fail"
			location.Message = err.Error()
			log.Println("Failed batch request: " + err.Error())
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedRequests()
			promMetrics.IncrementFailedBatchRequests()
			jsonLocation, _ := json.Marshal(&location)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(jsonLocation)
			return
		}

		//unmarshal request into slice
		var requests []ip_api.QueryIP
		err = json.Unmarshal(body,&requests)
		if err != nil {
			location.Status = "fail"
			location.Message = err.Error()
			log.Println("Failed batch request: " + err.Error())
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedRequests()
			promMetrics.IncrementFailedBatchRequests()
			jsonLocation, _ := json.Marshal(&location)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(jsonLocation)
			return
		}

		//validate the queries were actually passed
		if len(requests) == 0 {
			location.Status = "fail"
			location.Message = "no queries passed"
			log.Println("Failed batch request: no queries passed")
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedRequests()
			promMetrics.IncrementFailedBatchRequests()
			jsonLocation, _ := json.Marshal(&location)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(jsonLocation)
			return
		}

		//init slices
		var cachedLocations []ip_api.Location
		var cachedEcsLocations []EcsLocation
		var notCachedRequests []ip_api.QueryIP
		var notCachedRequestsMap = map[string]ip_api.QueryIP{}
		var cachedNewLocations []ip_api.Location
		var cachedNewEcsLocations []EcsLocation
		//First check for any requests that are in cache. Only want to forward non-cached requests
		var wg sync.WaitGroup
		wg.Add(len(requests))
		go func() {
			for _, request := range requests {
				//increment batch queries processed
				promMetrics.IncrementBatchQueriesProcessed()
				promMetrics.IncrementQueriesProcessed()

				var validatedSubFields string
				//validate sub fields
				if request.Fields != "" {
					validatedSubFields, err = ip_api.ValidateFields(request.Fields)
				}

				//validate sub lang
				var validatedSubLang string
				if request.Lang != "" {
					validatedSubLang, err = ip_api.ValidateLang(request.Lang)
				}

				//init location
				var location ip_api.Location

				//If err on sub fields or sub lang set as failed query status with err in message
				if err != nil {
					location.Status = "fail"
					location.Message = err.Error()
					location.Query = request.Query
					cachedLocations = append(cachedLocations, location) //Even though they aren't cached, we don't want to execute these as they are bad
					log.Println("Failed batch query: " + request.Query + " " + err.Error())
					promMetrics.IncrementHandlerRequests("400")
					promMetrics.IncrementFailedQueries()
					promMetrics.IncrementFailedBatchQueries()
				} else if request.Query == "" {
					location.Status = "fail"
					location.Message = "request is blank"
					location.Query = request.Query
					cachedLocations = append(cachedLocations, location) //Even though they aren't cached, we don't want to execute these as they are bad
					log.Println("Failed batch query: " + request.Query + " request is blank")
					promMetrics.IncrementHandlerRequests("400")
					promMetrics.IncrementFailedQueries()
					promMetrics.IncrementFailedBatchQueries()
				} else {
					//Check cache for ip
					var location *ip_api.Location
					var found bool

					if validatedSubFields != "" && validatedSubLang != "" {
						location, found, err = cache.GetLocation(request.Query + validatedSubLang,validatedSubFields)
						if err != nil {
							log.Println(err)
						}
					} else if validatedSubFields != "" {
						location, found, err = cache.GetLocation(request.Query + validatedLang,validatedSubFields)
						if err != nil {
							log.Println(err)
						}
					} else if validatedSubLang != "" {
						location, found, err = cache.GetLocation(request.Query + validatedSubLang,validatedFields)
						if err != nil {
							log.Println(err)
						}
					} else {
						location, found, err = cache.GetLocation(request.Query + validatedLang,validatedFields)
						if err != nil {
							log.Println(err)
						}
					}


					//if found in cache add to cached request list
					if found {
						promMetrics.IncrementCacheHits()
						promMetrics.IncrementSuccessfulQueries()
						promMetrics.IncrementSuccessfulBatchQueries()
						if LoadedConfig.Debugging {
							log.Println("Found: " + request.Query + " in cache.")
						}
						if !ecsBool {
							cachedLocations = append(cachedLocations, *location)
						} else {
							ecsLocation := EcsLocation{
								Status:        location.Status,
								Message:       location.Message,
								Continent:     location.Continent,
								ContinentCode: location.ContinentCode,
								Country:       location.Country,
								CountryCode:   location.CountryCode,
								Region:        location.Region,
								RegionName:    location.RegionName,
								City:          location.City,
								District:      location.District,
								ZIP:           location.ZIP,
								Lat:           location.Lat,
								Lon:           location.Lon,
								Timezone:      location.Timezone,
								Currency:      location.Currency,
								ISP:           location.ISP,
								Org:           location.Org,
								AS:            location.AS,
								ASName:        location.ASName,
								Reverse:       location.Reverse,
								Mobile:        location.Mobile,
								Proxy:         location.Proxy,
								Hosting:       location.Hosting,
								Query:         location.Query,
							}
							cachedEcsLocations = append(cachedEcsLocations, ecsLocation)
						}
					} else {
						//if not found in cache add to not cache request list
						promMetrics.IncrementQueriesForwarded()
						//add request to map for later lookups
						notCachedRequestsMap[request.Query] = request

						//set fields to all so that everything is stored in cache
						if request.Fields != "" {
							request.Fields = strings.Join(ip_api.AllowedAPIFields,",")
						}
						notCachedRequests = append(notCachedRequests, request)

					}
				}
				wg.Done()
			}
		}()

		wg.Wait()

		if len(notCachedRequests) > 0 {
			//Build batch request of non-cached requests
			batchQuery := ip_api.Query{
				Queries: notCachedRequests,
				Fields:  strings.Join(ip_api.AllowedAPIFields,","), //Execute query to IP API for all fields, handle field selection later
				Lang:    validatedLang,
			}

			//Execute batch request
			var notCachedLocations []ip_api.Location
			promMetrics.IncrementRequestsForwarded()
			notCachedLocations, err = ip_api.BatchQuery(batchQuery,key,"",LoadedConfig.Debugging)

			if err != nil {
				location.Status = "fail"
				location.Message = err.Error()
				log.Println("Failed batch request: " + err.Error())
				promMetrics.IncrementHandlerRequests("400")
				promMetrics.IncrementFailedRequests()
				promMetrics.IncrementFailedBatchRequests()
				jsonLocation, _ := json.Marshal(&location)
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write(jsonLocation)
				return
			}

			//Read non-cached requests and perform reverse lookups on successful requests
			if len(notCachedLocations) > 0 {
				//loop through not cached locations and get reverse records
				wg.Add(len(notCachedLocations))
				go func() {
					for _, location := range notCachedLocations {
						if location.Status == "success" {
							names, err := net.LookupAddr(location.Query)
							if len(names) > 0 && err == nil {
								location.Reverse = names[0]
							}
							//set lang value
							var lang string
							requestMap, ok := notCachedRequestsMap[location.Query]
							if ok {
								if requestMap.Lang != "" {
									lang = requestMap.Lang
								} else {
									lang = validatedLang
								}
							} else {
								lang = validatedLang
							}

							//Store non-cached location in cache and get back proper fields location
							_, err = cache.AddLocation(location.Query+lang, location, *LoadedConfig.Cache.SuccessAgeDuration)
							if err != nil {
								log.Println(err)
							}

							if LoadedConfig.Debugging {
								log.Println("Added Success: " + location.Query + lang + " in cache.")
							}

							//set fields value
							var fields string
							if requestMap, ok := notCachedRequestsMap[location.Query]; ok {
								fields = requestMap.Fields
							} else {
								fields = validatedFields
							}

							cachedLocation, _, err := cache.GetLocation(location.Query+lang, fields)
							if err != nil {
								log.Println(err)
							}
							if !ecsBool {
								cachedNewLocations = append(cachedNewLocations, *cachedLocation)
							} else {
								ecsLocation := EcsLocation{
									Status:        cachedLocation.Status,
									Message:       cachedLocation.Message,
									Continent:     cachedLocation.Continent,
									ContinentCode: cachedLocation.ContinentCode,
									Country:       cachedLocation.Country,
									CountryCode:   cachedLocation.CountryCode,
									Region:        cachedLocation.Region,
									RegionName:    cachedLocation.RegionName,
									City:          cachedLocation.City,
									District:      cachedLocation.District,
									ZIP:           cachedLocation.ZIP,
									Lat:           cachedLocation.Lat,
									Lon:           cachedLocation.Lon,
									Timezone:      cachedLocation.Timezone,
									Currency:      cachedLocation.Currency,
									ISP:           cachedLocation.ISP,
									Org:           cachedLocation.Org,
									AS:            cachedLocation.AS,
									ASName:        cachedLocation.ASName,
									Reverse:       cachedLocation.Reverse,
									Mobile:        cachedLocation.Mobile,
									Proxy:         cachedLocation.Proxy,
									Hosting:       cachedLocation.Hosting,
									Query:         cachedLocation.Query,
								}
								cachedNewEcsLocations = append(cachedNewEcsLocations, ecsLocation)
							}
							promMetrics.IncrementSuccessfulQueries()
							promMetrics.IncrementSuccessfulBatchQueries()
						} else {
							//set lang value
							var lang string
							requestMap, ok := notCachedRequestsMap[location.Query]
							if ok {
								if requestMap.Lang != "" {
									lang = requestMap.Lang
								} else {
									lang = validatedLang
								}
							} else {
								lang = validatedLang
							}

							//Store non-cached location in cache and get back proper fields location
							_, err = cache.AddLocation(location.Query+lang, location, *LoadedConfig.Cache.FailedAgeDuration)
							if err != nil {
								log.Println(err)
							}

							if LoadedConfig.Debugging {
								log.Println("Added Failed: " + location.Query + lang + " in cache.")
							}

							if !ecsBool {
								cachedNewLocations = append(cachedNewLocations, location)
							} else {
								ecsLocation := EcsLocation{
									Status:        location.Status,
									Message:       location.Message,
									Continent:     location.Continent,
									ContinentCode: location.ContinentCode,
									Country:       location.Country,
									CountryCode:   location.CountryCode,
									Region:        location.Region,
									RegionName:    location.RegionName,
									City:          location.City,
									District:      location.District,
									ZIP:           location.ZIP,
									Lat:           location.Lat,
									Lon:           location.Lon,
									Timezone:      location.Timezone,
									Currency:      location.Currency,
									ISP:           location.ISP,
									Org:           location.Org,
									AS:            location.AS,
									ASName:        location.ASName,
									Reverse:       location.Reverse,
									Mobile:        location.Mobile,
									Proxy:         location.Proxy,
									Hosting:       location.Hosting,
									Query:         location.Query,
								}
								cachedNewEcsLocations = append(cachedNewEcsLocations, ecsLocation)
							}
							log.Println("Failed query: " + location.Query)
							promMetrics.IncrementFailedQueries()
							promMetrics.IncrementFailedBatchQueries()
						}
						wg.Done()
					}
				}()
			}
			wg.Wait()
		}

		//Merge new requests with cached requests and return all
		if !ecsBool {
			if len(cachedNewLocations) > 0 {
				cachedLocations = append(cachedLocations, cachedNewLocations...)
			}
		} else {
			if len(cachedNewEcsLocations) > 0 {
				cachedEcsLocations = append(cachedEcsLocations, cachedNewEcsLocations...)
			}
		}


		//return query
		if !ecsBool {
			jsonLocation, _ := json.Marshal(cachedLocations)
			promMetrics.IncrementHandlerRequests("200")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonLocation)
		} else {
			jsonLocation, _ := json.Marshal(cachedEcsLocations)
			promMetrics.IncrementHandlerRequests("200")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonLocation)
		}
		return
	} else {
		if r.URL.Path != "/json/" && r.URL.Path != "/batch" && r.URL.Path != "/metrics" {
			location.Status = "fail"
			location.Message = "/batch endpoint only supports POST requests."
			log.Println("/batch endpoint only supports POST requests.")
			promMetrics.IncrementHandlerRequests("404")
			jsonLocation, _ := json.Marshal(&location)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write(jsonLocation)
			return
		}
	}
}

func ipAIPProxy(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/json/" && r.URL.Path != "/batch" && r.URL.Path != "/metrics" {
		var location ip_api.Location
		location.Status = "fail"
		location.Message = "server only supports GET (/json/ endpoint) and POST (/batch endpoint) requests."
		log.Println("404, server only supports GET (/json/ endpoint) and POST (/batch endpoint) requests.")
		promMetrics.IncrementHandlerRequests("404")
		jsonLocation, _ := json.Marshal(&location)
		w.Header().Add("Content-Type","application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write(jsonLocation)
		return
	}
}