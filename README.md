# ip-api-proxy

A third party Golang application which acts as a proxy for [IP-API's](http://ip-api.com/) API.

The goal of this proxy is to allow for a localized cache of requests to allow for the effective handling of hundreds of thousands, millions, or even tens of millions of requests a day. With the regular IP-API API, requests require a couple of milliseconds to complete (mainly from having to go over the Internet). With this proxy, the goal is to reduce the request times of redundant requests substantially, thus reducing total processing time of whatever application needs to perform requests.

The proxy API was designed to work the same way as IP-API's API, so that it could be easily integrated into any application flow that is already using it.

The only difference with value returns with the proxy API is that when a query fails it has the potential to return different message values then the standard IP-API API (though it will still return in the same IP-API format).

## Important Notes

1. This proxy is not intended to bypass IP-API's request limit of [150 requests per minute](http://ip-api.com/docs/api:json) on the free API URL. In fact there are no checks in this application to make sure that you never hit this limit, it just goes full throttle all the time. If you need to make more than 150 requests per minute, just buy the Pro service, its inexpensive.
2. Batch requests are handled differently with this proxy then you would expect when compared to the normal [IP-API batch](http://ip-api.com/docs/api:batch) request. This proxy will break apart a batch request it receives and execute each request individually. This means that if you are using the free API URL, you need to be conscious of how many requests will actually be made from a request.

## Install
### Build from Source

```
$ mkdir -p $GOPATH/src/github.com/BenB196/ip-api-proxy
$ cd $GOPATH/src/github.com/BenB196/ip-api-proxy
$ git clone github.com/BenB196/ip-api-proxy.git
$ cd ip-api-proxy
$ env GOOS=desired_os GOARCH=design_architecture go build -mod vendor -o /path/to/output/location #This command varies slightly based off of OS.
$ /path/to/output/location/ip-api-proxy --config=/path/to/config.json
```

### Precompiled binaries

### Docker
TODO add this

## Configuration

This proxy accepts a json Config file.

You can specify the location of the config file by passing the --config flag when running the application (ex: --config=/path/to/config.json).

If you do not specify a config file location, the application will default to trying to read ./config.json

An example config file can be found [here](docs/example_config.json).

### Config settings

```
{
  "cache": {
    "persist": false,       #If this is set to true, then the cache will be periodically written to disk, so that it can be read in the event of an app restart. Default: false
    "cleanInterval": "30m", #This is the interval that the proxy will go through and clean up any stale results from the cache which have expired. Default: 30m
    "writeInterval": "30m", #This is the interval that the cache is written to disk. Default: 30m, only works if persist == true.
    "writeLocation": "",    #This is the location where the cache will be written to disk. Defaul: working directory, only works if persist == true.
    "age": "24h"            #This is the age that a result is given, after which the result is marked as stale. Default: 24h
  },
  "port": 8080,             #This is the port which the application listens on. Default: 8080
  "apiKey": "",             #This is the API for using IP-API's pro API. Default: "", resorts to using the free API
  "prometheus": {
    "enabled": false        #This determines whether the Prometheus metrics endpoint is active. Default: false
  }
}
```

## Prometheus Integration

This proxy has been designed to support [Prometheus](https://prometheus.io/) metrics on the /metrics endpoint (ex: localhost:8080/metrics) 

The following are the currently supported metrics outside of the standard Golang metrics which Prometheus natively adds.

```
# HELP ip_api_proxy_cache_hits_total The total number of times that cache has served up a request
# TYPE ip_api_proxy_cache_hits_total counter
ip_api_proxy_cache_hits_total 0
# HELP ip_api_proxy_failed_queries_total The total number of failed queries
# TYPE ip_api_proxy_failed_queries_total counter
ip_api_proxy_failed_queries_total 0
# HELP ip_api_proxy_handler_requests_total Total number of requests by HTTP status code
# TYPE ip_api_proxy_handler_requests_total counter
ip_api_proxy_handler_requests_total{code="200"} 0
ip_api_proxy_handler_requests_total{code="400"} 0
ip_api_proxy_handler_requests_total{code="404"} 0
# HELP ip_api_proxy_queries_cached_total The total number of queries that have been cached locally
# TYPE ip_api_proxy_queries_cached_total counter
ip_api_proxy_queries_cached_total 0
# HELP ip_api_proxy_queries_forwarded_total The total number of queries forwarded to the IP API, API
# TYPE ip_api_proxy_queries_forwarded_total counter
ip_api_proxy_queries_forwarded_total 0
# HELP ip_api_proxy_queries_in_cache The current number of unique queries in the cache currently
# TYPE ip_api_proxy_queries_in_cache gauge
ip_api_proxy_queries_in_cache 0
# HELP ip_api_proxy_queries_total The total number of queries processed
# TYPE ip_api_proxy_queries_total counter
ip_api_proxy_queries_total 0
# HELP ip_api_proxy_successful_queries_total The total number of successfully fulfilled queries
# TYPE ip_api_proxy_successful_queries_total counter
ip_api_proxy_successful_queries_total 0
```

If you have ideas for other metrics which you feel would be useful, please let me know.