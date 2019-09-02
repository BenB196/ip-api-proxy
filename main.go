package main

import (
	"encoding/json"
	"fmt"
	"ip-api-go-pkg"
	"ip-api-proxy/cache"
	"ip-api-proxy/ipAPI"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

func main()  {
	http.HandleFunc("/",ipAIPProxy)

	var clearCacheWg sync.WaitGroup

	//Clear cache interval
	clearCacheTimeTicker := time.NewTicker(10 * time.Second) //TODO turn this duration into a variable
	clearCacheWg.Add(1)
	go func() {
		for {
			select {
			case <-clearCacheTimeTicker.C:
				log.Println("Clearing Cache")
				go cache.CleanUpCache()
			}
			defer clearCacheWg.Done()
		}
	}()

	//Listen on port
	log.Println("Starting server...")
	if err := http.ListenAndServe(":8080",nil); err != nil {
		log.Fatal(err)
	}
}

func ipAIPProxy(w http.ResponseWriter, r *http.Request) {
	//set content type
	w.Header().Set("Content-Type","application/json")

	//check to make sure only support sub pages are being used
	if r.URL.Path != "/json/" && !strings.Contains(r.URL.Path,"/json/")  && r.URL.Path != "/batch" {
		http.Error(w,"404 not found.",http.StatusNotFound)
		return
	}

	switch r.Method {
	case "GET":
		//Check to make sure that only json end point is getting GET requests
		if r.URL.Path != "/json/" && !strings.Contains(r.URL.Path,"/json/") {
			http.Error(w,"400 GET requests only supported for /json/.",http.StatusBadRequest)
			return
		}

		//check to make sure that there are only 2 or less / in URL
		if strings.Count(r.URL.Path,"/") > 2 {
			http.Error(w,"400 expected one (1) or (2) \"/\" but got more.",http.StatusNotFound)
			return
		}

		//get fields values
		fields, ok := r.URL.Query()["fields"]

		if !ok && len(fields) > 0 {
			http.Error(w,"400 invalid fields value.",http.StatusBadRequest)
			return
		}

		//get lang value
		lang, ok := r.URL.Query()["lang"]

		if !ok && len(lang) > 0 {
			http.Error(w,"400 invalid lang value.",http.StatusBadRequest)
			return
		}

		//validate fields
		validatedFields, err := ipAPI.ValidateFields(fields)

		if err != nil {
			http.Error(w,"400 " + err.Error(),http.StatusBadRequest)
			return
		}

		//validate lang
		validatedLang, err := ipAPI.ValidateLang(lang[0])

		if err != nil {
			http.Error(w, "400 " + err.Error(),http.StatusBadRequest)
			return
		}

		//Get ip address
		ip := ipAPI.IPDNSRegexp.FindString(r.URL.Path)

		//Check cache for ip
		location, found := cache.GetLocation(ip,validatedFields)

		//If ip found in cache return cached value
		if found {
			log.Println("Found: " + ip + " in cache.")
			jsonLocation, _ :=json.Marshal(location)
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
			Fields:ipAPI.AllowedAPIFields, //Execute query to IP API for all fields, handle field selection later
			Lang:validatedLang,
		}

		//execute query
		location, err = ip_api.SingleQuery(query,"","") //TODO turn apiKey into a variable

		if err != nil {
			http.Error(w,"400 " + err.Error(),http.StatusBadRequest)
			return
		}

		//Add to cache if successful query
		if location.Status == "success" {
			log.Println("Added: " + ip + " to cache.")
			cache.AddLocation(ip,location,10 * time.Second) //TODO turn duration into a variable
			//Re-get query with specified fields
			location, _ = cache.GetLocation(ip,validatedFields)
		}

		//return query
		jsonLocation, _ := json.Marshal(&location)
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(jsonLocation)
		if err != nil {
			log.Fatal(err)
		}
		return
	case "POST":
		//check to make sure that only batch end point is getting POST requests
		if r.URL.Path != "/batch" {
			http.Error(w,"400 POST requests only supported for /batch.",http.StatusBadRequest)
			return
		}
	default:
		_, err := fmt.Fprintf(w, "Error, server only supports GET and POST requests.")
		if err != nil {
			log.Println(err)
		}
	}
}