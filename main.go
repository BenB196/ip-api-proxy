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

var IPDNSRegexp = regexp.MustCompile(`(((25[0-5]|(2[0-4]|1?[0-9])?[0-9])\.){3}(25[0-5]|(2[0-4]|1?[0-9])?[0-9])|(([a-zA-Z])|([a-zA-Z][a-zA-Z])|([a-zA-Z][0-9])|([0-9][a-zA-Z])|([a-zA-Z0-9][a-zA-Z0-9-_]{1,61}[a-zA-Z0-9]))\.([a-zA-Z]{2,6}|[a-zA-Z0-9-]{2,30}\.[a-zA-Z]{2,3})|(([0-9a-fA-F]{1,4}:){7,7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:)|fe80:(:[0-9a-fA-F]{0,4}){0,4}%[0-9a-zA-Z]+|::(ffff(:0{1,4})?:)?((25[0-5]|(2[0-4]|1?[0-9])?[0-9])\.){3}(25[0-5]|(2[0-4]|1?[0-9])?[0-9])|([0-9a-fA-F]{1,4}:){1,4}:((25[0-5]|(2[0-4]|1?[0-9])?[0-9])\.){3}(25[0-5]|(2[0-4]|1?[0-9])?[0-9])))`)

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

	var clearCacheWg sync.WaitGroup

	//Clear cache interval
	clearCacheDuration,_ := time.ParseDuration(LoadedConfig.Cache.CleanInterval)
	clearCacheTimeTicker := time.NewTicker(clearCacheDuration)
	clearCacheWg.Add(1)
	go func() {
		for {
			select {
			case <-clearCacheTimeTicker.C:
				go cache.CleanUpCache()
			}
			defer clearCacheWg.Done()
		}
	}()

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
	log.Println("Starting server...")
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

	//Get cacheAge duration
	cacheAge, _ := time.ParseDuration(LoadedConfig.Cache.Age)

	if r.Method == "GET" {
		//check to make sure that there are only 2 or less / in URL
		if strings.Count(r.URL.Path,"/") > 2 {
			location.Status = "fail"
			location.Message = "400 expected one (1) or (2) \"/\" but got more."
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedRequests()
			promMetrics.IncrementFailedSingleRequests()
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusNotFound)
			return
		}

		//get fields values
		fields, ok := r.URL.Query()["fields"]
		if !ok && len(fields) > 0 {
			location.Status = "fail"
			location.Message = "400 invalid fields value."
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedRequests()
			promMetrics.IncrementFailedSingleRequests()
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusBadRequest)
			return
		}

		//get lang value
		lang, ok := r.URL.Query()["lang"]
		if !ok && len(lang) > 0 {
			location.Status = "fail"
			location.Message = "400 invalid lang value."
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedRequests()
			promMetrics.IncrementFailedSingleRequests()
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusBadRequest)
			return
		}

		//validate fields
		var validatedFields string
		if len(fields) > 0 {
			validatedFields, err = ip_api.ValidateFields(fields[0])

			if err != nil {
				location.Status = "fail"
				location.Message = "400 " + err.Error()
				promMetrics.IncrementHandlerRequests("400")
				promMetrics.IncrementFailedRequests()
				promMetrics.IncrementFailedSingleRequests()
				jsonLocation, _ := json.Marshal(&location)
				http.Error(w,string(jsonLocation),http.StatusBadRequest)
				return
			}
		}

		//validate lang
		var validatedLang string
		if len(lang) > 0 {
			validatedLang, err = ip_api.ValidateLang(lang[0])

			if err != nil {
				location.Status = "fail"
				location.Message = "400 " + err.Error()
				promMetrics.IncrementHandlerRequests("400")
				promMetrics.IncrementFailedRequests()
				promMetrics.IncrementFailedSingleRequests()
				jsonLocation, _ := json.Marshal(&location)
				http.Error(w,string(jsonLocation),http.StatusBadRequest)
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
			location.Message = "400 request is blank"
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedRequests()
			promMetrics.IncrementFailedSingleRequests()
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusBadRequest)
			return
		}

		//Check cache for ip
		location, found := cache.GetLocation(ip + validatedLang,validatedFields)

		//If ip found in cache return cached value
		if found {
			log.Println("Found: " + ip + " in cache.")
			promMetrics.IncrementHandlerRequests("200")
			promMetrics.IncrementCacheHits()
			promMetrics.IncrementSuccessfulQueries()
			promMetrics.IncrementSuccessfulSingeQueries()
			jsonLocation, err :=json.Marshal(location)
			w.WriteHeader(http.StatusOK)
			_, err = w.Write(jsonLocation)
			if err != nil {
				log.Fatal(err)
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
		newLocation, err := ip_api.SingleQuery(query,key,"")

		if err != nil {
			location.Status = "fail"
			location.Message = "400 " + err.Error()
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedRequests()
			promMetrics.IncrementFailedSingleRequests()
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusBadRequest)
			return
		}

		//Add to cache if successful request
		if newLocation.Status == "success" {
			log.Println("Added: " + ip + validatedLang + " to cache.")
			promMetrics.IncrementHandlerRequests("200")
			cache.AddLocation(ip + validatedLang,newLocation,cacheAge)
			//Re-get request with specified fields
			newLocation, _ = cache.GetLocation(ip + validatedLang,validatedFields)
			promMetrics.IncrementSuccessfulQueries()
			promMetrics.IncrementSuccessfulSingeQueries()
		}

		//if request failed, increment 400 and fail request counter
		if newLocation.Status == "failed" {
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedQueries()
			promMetrics.IncrementFailedSingleQueries()
		}

		//return query
		jsonLocation, _ := json.Marshal(&newLocation)
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(jsonLocation)
		if err != nil {
			log.Fatal(err)
		}
		return
	} else {
		if r.URL.Path != "/json/" && r.URL.Path != "/batch" && r.URL.Path != "/metrics" {
			location.Status = "fail"
			location.Message = "404, /json/ endpoint only supports GET requests."
			promMetrics.IncrementHandlerRequests("404")
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w, string(jsonLocation), http.StatusBadRequest)
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

	//Get cacheAge duration
	cacheAge, _ := time.ParseDuration(LoadedConfig.Cache.Age)

	if r.Method == "POST" {
		//check to make sure that there are only 1 or less / in URL
		if strings.Count(r.URL.Path,"/") > 1 {
			location.Status = "fail"
			location.Message = "400 expected one (1) \"/\" but got more."
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedRequests()
			promMetrics.IncrementFailedBatchRequests()
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusNotFound)
			return
		}

		//get fields values
		fields, ok := r.URL.Query()["fields"]
		if !ok && len(fields) > 0 {
			location.Status = "fail"
			location.Message = "400 invalid fields value."
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedRequests()
			promMetrics.IncrementFailedBatchRequests()
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusBadRequest)
			return
		}

		//get lang value
		lang, ok := r.URL.Query()["lang"]
		if !ok && len(lang) > 0 {
			location.Status = "fail"
			location.Message = "400 invalid lang value."
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedRequests()
			promMetrics.IncrementFailedBatchRequests()
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusBadRequest)
			return
		}

		//validate fields
		var validatedFields string
		if len(fields) > 0 {
			validatedFields, err = ip_api.ValidateFields(fields[0])

			if err != nil {
				location.Status = "fail"
				location.Message = "400 " + err.Error()
				promMetrics.IncrementHandlerRequests("400")
				promMetrics.IncrementFailedRequests()
				promMetrics.IncrementFailedBatchRequests()
				jsonLocation, _ := json.Marshal(&location)
				http.Error(w,string(jsonLocation),http.StatusBadRequest)
				return
			}
		}

		//validate lang
		var validatedLang string
		if len(lang) > 0 {
			validatedLang, err = ip_api.ValidateLang(lang[0])

			if err != nil {
				location.Status = "fail"
				location.Message = "400 " + err.Error()
				promMetrics.IncrementHandlerRequests("400")
				promMetrics.IncrementFailedRequests()
				promMetrics.IncrementFailedBatchRequests()
				jsonLocation, _ := json.Marshal(&location)
				http.Error(w,string(jsonLocation),http.StatusBadRequest)
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
			location.Message = "400 " + err.Error()
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedRequests()
			promMetrics.IncrementFailedBatchRequests()
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusBadRequest)
			return
		}

		//unmarshal request into slice
		var requests []ip_api.QueryIP
		err = json.Unmarshal(body,&requests)
		if err != nil {
			location.Status = "fail"
			location.Message = "400 " + err.Error()
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedRequests()
			promMetrics.IncrementFailedBatchRequests()
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusBadRequest)
			return
		}

		//validate the queries were actually passed
		if len(requests) == 0 {
			location.Status = "fail"
			location.Message = "400 no queries passed"
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedRequests()
			promMetrics.IncrementFailedBatchRequests()
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusBadRequest)
			return
		}

		//init slices
		var cachedLocations []ip_api.Location
		var notCachedRequests []ip_api.QueryIP
		var notCachedRequestsMap = map[string]ip_api.QueryIP{}
		var cachedNewLocations []ip_api.Location
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
					location.Message = "400 " + err.Error()
					location.Query = request.Query
					cachedLocations = append(cachedLocations, location) //Even though they aren't cached, we don't want to execute these as they are bad
					promMetrics.IncrementHandlerRequests("400")
					promMetrics.IncrementFailedQueries()
					promMetrics.IncrementFailedBatchQueries()
				} else if request.Query == "" {
					location.Status = "fail"
					location.Message = "400 request is blank"
					location.Query = request.Query
					cachedLocations = append(cachedLocations, location) //Even though they aren't cached, we don't want to execute these as they are bad
					promMetrics.IncrementHandlerRequests("400")
					promMetrics.IncrementFailedQueries()
					promMetrics.IncrementFailedBatchQueries()
				} else {
					//Check cache for ip
					var location ip_api.Location
					var found bool

					if validatedSubFields != "" && validatedSubLang != "" {
						location, found = cache.GetLocation(request.Query + validatedSubLang,validatedSubFields)
					} else if validatedSubFields != "" {
						location, found = cache.GetLocation(request.Query + validatedLang,validatedSubFields)
					} else if validatedSubLang != "" {
						location, found = cache.GetLocation(request.Query + validatedSubLang,validatedFields)
					} else {
						location, found = cache.GetLocation(request.Query + validatedLang,validatedFields)
					}


					//if found in cache add to cached request list
					if found {
						promMetrics.IncrementCacheHits()
						promMetrics.IncrementSuccessfulQueries()
						promMetrics.IncrementSuccessfulBatchQueries()
						log.Println("Found: " + request.Query + " in cache.")
						cachedLocations = append(cachedLocations, location)
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
			notCachedLocations, err = ip_api.BatchQuery(batchQuery,key,"")

			if err != nil {
				location.Status = "fail"
				location.Message = "400 " + err.Error()
				promMetrics.IncrementHandlerRequests("400")
				promMetrics.IncrementFailedRequests()
				promMetrics.IncrementFailedBatchRequests()
				jsonLocation, _ := json.Marshal(&location)
				http.Error(w,string(jsonLocation),http.StatusBadRequest)
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
							cache.AddLocation(location.Query+lang, location, cacheAge)
							log.Println("Added: " + location.Query + lang + " in cache.")

							//set fields value
							var fields string
							if requestMap, ok := notCachedRequestsMap[location.Query]; ok {
								fields = requestMap.Fields
							} else {
								fields = validatedFields
							}

							cachedLocation, _ := cache.GetLocation(location.Query+lang, fields)
							cachedNewLocations = append(cachedNewLocations, cachedLocation)
							promMetrics.IncrementSuccessfulQueries()
							promMetrics.IncrementSuccessfulBatchQueries()
						} else {
							cachedNewLocations = append(cachedNewLocations, location)
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
		if len(cachedNewLocations) > 0 {
			cachedLocations = append(cachedLocations, cachedNewLocations...)
		}

		//return query
		jsonLocation, _ := json.Marshal(cachedLocations)
		promMetrics.IncrementHandlerRequests("200")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(jsonLocation)
		if err != nil {
			log.Fatal(err)
		}
		return
	} else {
		if r.URL.Path != "/json/" && r.URL.Path != "/batch" && r.URL.Path != "/metrics" {
			location.Status = "fail"
			location.Message = "404, /batch endpoint only supports POST requests."
			promMetrics.IncrementHandlerRequests("404")
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w, string(jsonLocation), http.StatusBadRequest)
			return
		}
	}
}

func ipAIPProxy(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/json/" && r.URL.Path != "/batch" && r.URL.Path != "/metrics" {
		var location ip_api.Location
		location.Status = "fail"
		location.Message = "404, server only supports GET (/json/ endpoint) and POST (/batch endpoint) requests."
		promMetrics.IncrementHandlerRequests("404")
		jsonLocation, _ := json.Marshal(&location)
		http.Error(w,string(jsonLocation),http.StatusBadRequest)
		return
	}
}