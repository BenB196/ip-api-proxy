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
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

//Init config globally
var LoadedConfig = config.Config{}

var IPDNSRegexp = regexp.MustCompile(`((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)(\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)){3}|(([a-zA-Z])|([a-zA-Z][a-zA-Z])|([a-zA-Z][0-9])|([0-9][a-zA-Z])|([a-zA-Z0-9][a-zA-Z0-9-_]{1,61}[a-zA-Z0-9]))\.([a-zA-Z]{2,6}|[a-zA-Z0-9-]{2,30}\.[a-zA-Z]{2,3}))`)

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
	//increment queries processed
	promMetrics.IncrementQueriesProcessed()
	//TODO add single increment

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
			location.Status = "failed"
			location.Message = "400 expected one (1) or (2) \"/\" but got more."
			promMetrics.IncrementHandlerRequests("400")
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusNotFound)
			return
		}

		//get fields values
		fields, ok := r.URL.Query()["fields"]
		if !ok && len(fields) > 0 {
			location.Status = "failed"
			location.Message = "400 invalid fields value."
			promMetrics.IncrementHandlerRequests("400")
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusBadRequest)
			return
		}

		//get lang value
		lang, ok := r.URL.Query()["lang"]
		if !ok && len(lang) > 0 {
			location.Status = "failed"
			location.Message = "400 invalid lang value."
			promMetrics.IncrementHandlerRequests("400")
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusBadRequest)
			return
		}

		//validate fields
		var validatedFields string
		if len(fields) > 0 {
			validatedFields, err = ip_api.ValidateFields(fields[0])

			if err != nil {
				location.Status = "failed"
				location.Message = "400 " + err.Error()
				promMetrics.IncrementHandlerRequests("400")
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
				location.Status = "failed"
				location.Message = "400 " + err.Error()
				promMetrics.IncrementHandlerRequests("400")
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
			location.Status = "failed"
			location.Message = "400 request is blank"
			promMetrics.IncrementHandlerRequests("400")
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusBadRequest)
			return
		}

		//Check cache for ip
		location, found := cache.GetLocation(ip,validatedFields)

		//If ip found in cache return cached value
		if found {
			log.Println("Found: " + ip + " in cache.")
			promMetrics.IncrementHandlerRequests("200")
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
		promMetrics.IncrementQueriesForwarded()
		newLocation, err := ip_api.SingleQuery(query,key,"")

		if err != nil {
			location.Status = "failed"
			location.Message = "400 " + err.Error()
			promMetrics.IncrementHandlerRequests("400")
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusBadRequest)
			return
		}

		//Add to cache if successful request
		if newLocation.Status == "success" {
			log.Println("Added: " + ip + " to cache.")
			promMetrics.IncrementHandlerRequests("200")
			cache.AddLocation(ip,newLocation,cacheAge)
			//Re-get request with specified fields
			promMetrics.IncrementSuccessfulQueries()
			location, _ = cache.GetLocation(ip,validatedFields)
		}

		//if request failed, increment 400 and fail request counter
		if newLocation.Status == "failed" {
			promMetrics.IncrementHandlerRequests("400")
			promMetrics.IncrementFailedQueries()
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
		location.Status = "failed"
		location.Message = "404, /json/ endpoint only supports GET requests."
		promMetrics.IncrementHandlerRequests("404")
		jsonLocation, _ := json.Marshal(&location)
		http.Error(w,string(jsonLocation),http.StatusBadRequest)
		return
	}
}

func ipAPIBatch(w http.ResponseWriter, r *http.Request) {
	//increment queries processed
	promMetrics.IncrementQueriesProcessed()
	//TODO add batch increment

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
			location.Status = "failed"
			location.Message = "400 expected one (1) \"/\" but got more."
			promMetrics.IncrementHandlerRequests("400")
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusNotFound)
			return
		}

		//get fields values
		fields, ok := r.URL.Query()["fields"]
		if !ok && len(fields) > 0 {
			location.Status = "failed"
			location.Message = "400 invalid fields value."
			promMetrics.IncrementHandlerRequests("400")
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusBadRequest)
			return
		}

		//get lang value
		lang, ok := r.URL.Query()["lang"]
		if !ok && len(lang) > 0 {
			location.Status = "failed"
			location.Message = "400 invalid lang value."
			promMetrics.IncrementHandlerRequests("400")
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusBadRequest)
			return
		}

		//validate fields
		var validatedFields string
		if len(fields) > 0 {
			validatedFields, err = ip_api.ValidateFields(fields[0])

			if err != nil {
				location.Status = "failed"
				location.Message = "400 " + err.Error()
				promMetrics.IncrementHandlerRequests("400")
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
				location.Status = "failed"
				location.Message = "400 " + err.Error()
				promMetrics.IncrementHandlerRequests("400")
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
			location.Status = "failed"
			location.Message = "400 " + err.Error()
			promMetrics.IncrementHandlerRequests("400")
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusBadRequest)
			return
		}

		//unmarshal request into slice
		var queries []ip_api.QueryIP
		err = json.Unmarshal(body,&queries)
		if err != nil {
			location.Status = "failed"
			location.Message = "400 " + err.Error()
			promMetrics.IncrementHandlerRequests("400")
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusBadRequest)
			return
		}

		var locations []ip_api.Location

		//validate the queries were actually passed
		if len(queries) == 0 {
			location.Status = "failed"
			location.Message = "400 no queries passed"
			promMetrics.IncrementHandlerRequests("400")
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusBadRequest)
			return
		}

		//TODO first check for any requests that are in cache. Only want to forward non-cached requests

		//TODO build batch request of non-cached requests

		//TODO read non-cached requests and perform reverse lookups on successful requests

		//TODO store non-cached requests in cache and get back proper fields requests

		//TODO merge new requests with cached requests and return all
	} else {
		location.Status = "failed"
		location.Message = "404, /batch endpoint only supports POST requests."
		promMetrics.IncrementHandlerRequests("404")
		jsonLocation, _ := json.Marshal(&location)
		http.Error(w,string(jsonLocation),http.StatusBadRequest)
		return
	}
}

func ipAIPProxy(w http.ResponseWriter, r *http.Request) {



	switch r.Method {
	case "POST":





















		//Loop through queries from post data and handle each query as a single query
		//var wg sync.WaitGroup
		//wg.Add(len(queries))
		//go func() {
		//	for _, query := range queries {
		//		defer wg.Done()
		//		//validate any sub fields
		//		if query.Fields != "" {
		//			validatedFields, err = ip_api.ValidateFields(query.Fields)
		//		}
//
		//		//validate any sub langs
		//		if query.Lang != "" {
		//			validatedLang, err = ip_api.ValidateLang(query.Lang)
		//		}
//
		//		//init location
		//		var location ip_api.Location
//
		//		//If err on sub fields or sub lang set as failed query status with err in message
		//		if err != nil {
		//			location.Status = "failed"
		//			location.Message = "400 " + err.Error()
		//			promMetrics.IncrementHandlerRequests("400")
		//		} else if query.Query == "" {
		//			location.Status = "failed"
		//			location.Message = "400 query request is blank"
		//			promMetrics.IncrementHandlerRequests("400")
		//		} else {
		//			//Check cache for ip
		//			location, found := cache.GetLocation(query.Query,validatedFields)
//
		//			//If ip found in cache return cached value
		//			if found {
		//				log.Println("Found: " + query.Query + " in cache.")
		//				promMetrics.IncrementHandlerRequests("200")
		//				//Append query to location list
		//				locations = append(locations, location)
		//			}
//
		//			//execute query
		//			if !found {
		//				//Build query
		//				queryStruct := ip_api.Query{
		//					Queries:[]ip_api.QueryIP{
		//						{Query:query.Query},
		//					},
		//					Fields:strings.Join(ip_api.AllowedAPIFields,","), //Execute query to IP API for all fields, handle field selection later
		//					Lang:validatedLang,
		//				}
//
		//				promMetrics.IncrementQueriesForwarded()
		//				location, err = ip_api.SingleQuery(queryStruct,key,"")
//
		//				if err != nil {
		//					location = ip_api.Location{}
		//					location.Status = "failed"
		//					location.Message = "400 " + err.Error()
		//					promMetrics.IncrementHandlerRequests("400")
		//				}
//
		//				//Add to cache if successful query
		//				if location.Status == "success" {
		//					log.Println("Added: " + query.Query + " to cache.")
		//					cache.AddLocation(query.Query,location,cacheAge)
		//					//Re-get query with specified fields
		//					promMetrics.IncrementHandlerRequests("200")
		//					promMetrics.IncrementSuccessfulQueries()
		//					location, _ = cache.GetLocation(query.Query,validatedFields)
		//				}
//
		//				if location.Status == "failed" {
		//					promMetrics.IncrementHandlerRequests("400")
		//					promMetrics.IncrementFailedQueries()
		//				}
//
		//				//Append query to location list
		//				locations = append(locations, location)
		//			}
		//		}
		//	}
		//}()
//
		//wg.Wait()
//
		////return query as an array
		//jsonLocations, _ := json.Marshal(&locations)
		//w.WriteHeader(http.StatusOK)
		//_, err = w.Write(jsonLocations)
		//if err != nil {
		//	log.Fatal(err)
		//}
		//return
	default:
		var location ip_api.Location
		location.Status = "failed"
		location.Message = "404, server only supports GET (/json/ endpoint) and POST (/batch endpoint) requests."
		promMetrics.IncrementHandlerRequests("404")
		jsonLocation, _ := json.Marshal(&location)
		http.Error(w,string(jsonLocation),http.StatusBadRequest)
		return
	}
}