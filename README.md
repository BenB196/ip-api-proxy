# ip-api-proxy

A third party Golang application which acts as a proxy for [IP-API's](http://ip-api.com/) API.

The goal of this proxy is to allow for a localized cache of requests to allow for the effective handling of hundreds of thousands, millions, or even tens of millions of requests a day. With the regular IP-API API, requests require a couple of milliseconds to complete (mainly from having to go over the Internet). With this proxy, the goal is to reduce the request times of redundant requests substantially, thus reducing total processing time of whatever application needs to perform requests.

The proxy API was designed to work the same way as IP-API's API, so that it could be easily integrated into any application flow that is already using it.

The only difference with value returns with the proxy API is that when a query fails it has the potential to return different message values then the standard IP-API API (though it will still return in the same IP-API format).

## Important Notes

1. This proxy is not intended to bypass IP-API's request limit of [150 requests per minute](http://ip-api.com/docs/api:json) on the free API URL. In fact there are no checks in this application to make sure that you never hit this limit, it just goes full throttle all the time. If you need to make more than 150 requests per minute, just buy the Pro service, its inexpensive.
2. Batch requests are handled differently with this proxy then you would expect when compared to the normal [IP-API batch](http://ip-api.com/docs/api:batch) request. This proxy will provide reverse records if you pass the reverse field value through a batch query.

## Install
### Build from Source

```
$ mkdir -p $GOPATH/src/github.com/BenB196/
$ cd $GOPATH/src/github.com/BenB196/
$ git clone https://github.com/BenB196/ip-api-proxy.git
$ cd ip-api-proxy
$ env GOOS=desired_os GOARCH=design_architecture go build -o /path/to/output/location #This command varies slightly based off of OS.
$ /path/to/output/location/ip-api-proxy --config=/path/to/config.json
```

### Precompiled binaries

These are found attached to each release. Current plan is to only release the Windows amd64 and Linux amd64 bins initially. More will be added eventually.

### Docker

This proxy is able to be run in a Docker container as well.

Pull:
```docker pull benb196/ip-api-proxy```

The container can either be run stateless (if persist == false in the config):

```
docker run -d -p <external_port>:<container_port> benb196/ip-api-proxy
```

If you want to persist the cache, then you can mount a volume to /root/.

```
docker run -d\
 -p <external_port>:<container_port>\
 -v /path/to/storage:/root\
 benb196/ip-api-proxy
```

#### Docker Compose (3)

```
version: "3"
services:
  ip-api-proxy:
    image: benb196/ip-api-proxy
    volumes:
      - /path/to/storage:/root
    ports:
      - 8080:8080
```

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
    "successAge": "24h",    #This is the age that a result is given for success, after which the result is marked as stale. Default: 24h
    "failedAge": "30m"      #This is the age that a result is given for failed, after which the result is marked as stale. Default: 30m
  },
  "port": 8080,             #This is the port which the application listens on. Default: 8080
  "debugging": true,        #This is used to log queries for debugging purposes
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
# HELP ip_api_proxy_batch_queries_processed_total The total number of batch queries processed
# TYPE ip_api_proxy_batch_queries_processed_total counter
ip_api_proxy_batch_queries_processed_total 0
# HELP ip_api_proxy_batch_requests_processed_total The total number of batch requests processed
# TYPE ip_api_proxy_batch_requests_processed_total counter
ip_api_proxy_batch_requests_processed_total 0
# HELP ip_api_proxy_cache_hits_total The total number of times that cache has served up a request
# TYPE ip_api_proxy_cache_hits_total counter
ip_api_proxy_cache_hits_total 0
# HELP ip_api_proxy_failed_batch_queries_total The total number of failed batch queries
# TYPE ip_api_proxy_failed_batch_queries_total counter
ip_api_proxy_failed_batch_queries_total 0
# HELP ip_api_proxy_failed_batch_requests_total The total number of failed batch requests
# TYPE ip_api_proxy_failed_batch_requests_total counter
ip_api_proxy_failed_batch_requests_total 0
# HELP ip_api_proxy_failed_queries_total The total number of failed queries
# TYPE ip_api_proxy_failed_queries_total counter
ip_api_proxy_failed_queries_total 0
# HELP ip_api_proxy_failed_requests_total The total number of failed requests
# TYPE ip_api_proxy_failed_requests_total counter
ip_api_proxy_failed_requests_total 0
# HELP ip_api_proxy_failed_single_queries_total The total number of failed single queries
# TYPE ip_api_proxy_failed_single_queries_total counter
ip_api_proxy_failed_single_queries_total 0
# HELP ip_api_proxy_failed_single_requests_total The total number of failed single requests
# TYPE ip_api_proxy_failed_single_requests_total counter
ip_api_proxy_failed_single_requests_total 0
# HELP ip_api_proxy_handler_requests_total Total number of requests by HTTP status code
# TYPE ip_api_proxy_handler_requests_total counter
ip_api_proxy_handler_requests_total{code="200"} 0
ip_api_proxy_handler_requests_total{code="400"} 0
ip_api_proxy_handler_requests_total{code="404"} 0
# HELP ip_api_proxy_queries_cached_total The total number of queries that have been cached locally
# TYPE ip_api_proxy_queries_cached_total counter
ip_api_proxy_queries_cached_total 0
# HELP ip_api_proxy_queries_forwarded_total The total number of queries forwarded to IP-API
# TYPE ip_api_proxy_queries_forwarded_total counter
ip_api_proxy_queries_forwarded_total 0
# HELP ip_api_proxy_queries_in_cache The current number of unique queries in the cache currently
# TYPE ip_api_proxy_queries_in_cache gauge
ip_api_proxy_queries_in_cache 0
# HELP ip_api_proxy_queries_total The total number of queries processed
# TYPE ip_api_proxy_queries_total counter
ip_api_proxy_queries_total 0
# HELP ip_api_proxy_requests_forwarded_total The total number of requests forwarded to IP-API
# TYPE ip_api_proxy_requests_forwarded_total counter
ip_api_proxy_requests_forwarded_total 0
# HELP ip_api_proxy_requests_total The total number of requests processed
# TYPE ip_api_proxy_requests_total counter
ip_api_proxy_requests_total 0
# HELP ip_api_proxy_single_queries_processed_total The total number of single queries processed
# TYPE ip_api_proxy_single_queries_processed_total counter
ip_api_proxy_single_queries_processed_total 0
# HELP ip_api_proxy_single_requests_processed_total The total number of single requests processed
# TYPE ip_api_proxy_single_requests_processed_total counter
ip_api_proxy_single_requests_processed_total 0
# HELP ip_api_proxy_successful_batch_queries_total The total number of successfully fulfilled batch queries
# TYPE ip_api_proxy_successful_batch_queries_total counter
ip_api_proxy_successful_batch_queries_total 0
# HELP ip_api_proxy_successful_queries_total The total number of successfully fulfilled queries
# TYPE ip_api_proxy_successful_queries_total counter
ip_api_proxy_successful_queries_total 0
# HELP ip_api_proxy_successful_single_queries_total The total number of successfully fulfilled single queries
# TYPE ip_api_proxy_successful_single_queries_total counter
ip_api_proxy_successful_single_queries_total 0
```

## Elastic Common Schema (ECS) Support

Support for outputting IP-API results in the [ECS Standard](https://www.elastic.co/guide/en/ecs/current/ecs-geo.html) has been added. In order to get results in this format, when making queries against the API use ecs=false in the HTTP request:

ex:
```
http://localhost:8080/batch?ecs=true
```

This will return an output that matches the [ECS Standard](https://www.elastic.co/guide/en/ecs/current/ecs-geo.html)

Note: There are additional fields returned by IP-API, that are not listed under [ECS Standard](https://www.elastic.co/guide/en/ecs/current/ecs-geo.html), if you want to index these fields, please make sure you're index template is designed to handle them.

If you have ideas for other metrics which you feel would be useful, please let me know.
