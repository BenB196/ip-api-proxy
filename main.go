package main

import (
	"encoding/json"
	"flag"
	"github.com/BenB196/ip-api-go-pkg"
	"github.com/BenB196/ip-api-proxy/cache"
	"github.com/BenB196/ip-api-proxy/config"
	"github.com/BenB196/ip-api-proxy/ipAPI"
	"github.com/BenB196/ip-api-proxy/promMetrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

//Init config globally
var LoadedConfig = config.Config{}

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

	http.HandleFunc("/",ipAIPProxy)

	if LoadedConfig.Prometheus.Enabled {
		//Start prometheus metrics end point
		http.Handle("/metrics",promhttp.Handler())
	}

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
		log.Fatal(err)
	}
}

func ipAIPProxy(w http.ResponseWriter, r *http.Request) {
	//set content type
	w.Header().Set("Content-Type","application/json")

	//init location variable
	location := ip_api.Location{}

	//check to make sure only support sub pages are being used
	if r.URL.Path != "/json/" && !strings.Contains(r.URL.Path,"/json/")  && r.URL.Path != "/batch" {
		location.Status = "failed"
		location.Message = "404 not found."
		promMetrics.IncrementHandlerRequests("404")
		jsonLocation, _ := json.Marshal(&location)
		http.Error(w,string(jsonLocation),http.StatusNotFound)
		return
	}

	cacheAge, _ := time.ParseDuration(LoadedConfig.Cache.Age)

	switch r.Method {
	case "GET":
		promMetrics.IncrementQueriesProcessed()
		//Check to make sure that only json end point is getting GET requests
		if r.URL.Path != "/json/" && !strings.Contains(r.URL.Path,"/json/") {
			location.Status = "failed"
			location.Message = "400 GET requests only supported for /json/."
			promMetrics.IncrementHandlerRequests("400")
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusBadRequest)
			return
		}

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
		var err error
		if len(fields) > 0 {
			validatedFields, err = ipAPI.ValidateFields(fields[0])

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
			validatedLang, err = ipAPI.ValidateLang(lang[0])

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
		ip := ipAPI.IPDNSRegexp.FindString(r.URL.Path)

		if ip == "" {
			location.Status = "failed"
			location.Message = "400 query request is blank"
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
			Fields:strings.Join(ipAPI.AllowedAPIFields,","), //Execute query to IP API for all fields, handle field selection later
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

		//Add to cache if successful query
		if newLocation.Status == "success" {
			log.Println("Added: " + ip + " to cache.")
			promMetrics.IncrementHandlerRequests("200")
			cache.AddLocation(ip,newLocation,cacheAge)
			//Re-get query with specified fields
			promMetrics.IncrementSuccessfulQueries()
			location, _ = cache.GetLocation(ip,validatedFields)
		}

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
	case "POST":
		promMetrics.IncrementQueriesProcessed()
		//check to make sure that only batch end point is getting POST requests
		if r.URL.Path != "/batch" {
			location.Status = "failed"
			location.Message = "400 POST requests only supported for /batch."
			promMetrics.IncrementHandlerRequests("400")
			jsonLocation, _ := json.Marshal(&location)
			http.Error(w,string(jsonLocation),http.StatusBadRequest)
			return
		}

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
		var err error
		if len(fields) > 0 {
			validatedFields, err = ipAPI.ValidateFields(fields[0])

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
			validatedLang, err = ipAPI.ValidateLang(lang[0])

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

		//Loop through queries from post data and handle each query as a single query
		var wg sync.WaitGroup
		wg.Add(len(queries))
		go func() {
			for _, query := range queries {
				defer wg.Done()
				//validate any sub fields
				if query.Fields != "" {
					validatedFields, err = ipAPI.ValidateFields(query.Fields)
				}

				//validate any sub langs
				if query.Lang != "" {
					validatedLang, err = ipAPI.ValidateLang(query.Lang)
				}

				//init location
				var location ip_api.Location

				//If err on sub fields or sub lang set as failed query status with err in message
				if err != nil {
					location.Status = "failed"
					location.Message = "400 " + err.Error()
					promMetrics.IncrementHandlerRequests("400")
				} else if query.Query == "" {
					location.Status = "failed"
					location.Message = "400 query request is blank"
					promMetrics.IncrementHandlerRequests("400")
				} else {
					//Check cache for ip
					location, found := cache.GetLocation(query.Query,validatedFields)

					//If ip found in cache return cached value
					if found {
						log.Println("Found: " + query.Query + " in cache.")
						promMetrics.IncrementHandlerRequests("200")
						//Append query to location list
						locations = append(locations, location)
					}

					//execute query
					if !found {
						//Build query
						queryStruct := ip_api.Query{
							Queries:[]ip_api.QueryIP{
								{Query:query.Query},
							},
							Fields:strings.Join(ipAPI.AllowedAPIFields,","), //Execute query to IP API for all fields, handle field selection later
							Lang:validatedLang,
						}

						promMetrics.IncrementQueriesForwarded()
						location, err = ip_api.SingleQuery(queryStruct,key,"")

						if err != nil {
							location = ip_api.Location{}
							location.Status = "failed"
							location.Message = "400 " + err.Error()
							promMetrics.IncrementHandlerRequests("400")
						}

						//Add to cache if successful query
						if location.Status == "success" {
							log.Println("Added: " + query.Query + " to cache.")
							cache.AddLocation(query.Query,location,cacheAge)
							//Re-get query with specified fields
							promMetrics.IncrementHandlerRequests("200")
							promMetrics.IncrementSuccessfulQueries()
							location, _ = cache.GetLocation(query.Query,validatedFields)
						}

						if location.Status == "failed" {
							promMetrics.IncrementHandlerRequests("400")
							promMetrics.IncrementFailedQueries()
						}

						//Append query to location list
						locations = append(locations, location)
					}
				}
			}
		}()

		wg.Wait()

		//return query as an array
		jsonLocations, _ := json.Marshal(&locations)
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(jsonLocations)
		if err != nil {
			log.Fatal(err)
		}
		return
	default:
		var location ip_api.Location
		location.Status = "failed"
		location.Message = "404, server only supports GET and POST requests."
		promMetrics.IncrementHandlerRequests("404")
		jsonLocation, _ := json.Marshal(&location)
		http.Error(w,string(jsonLocation),http.StatusBadRequest)
		return
	}
}