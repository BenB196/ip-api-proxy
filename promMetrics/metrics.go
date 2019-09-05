package promMetrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	queriesProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ip_api_proxy_queries_total",
		Help: "The total number of queries processed",
	})
	queriesForwarded = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ip_api_proxy_queries_forwarded_total",
		Help: "The total number of queries forwarded to the IP API, API",
	})
	queriesCachedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ip_api_proxy_queries_cached_total",
		Help: "The total number of queries that have been cached locally",
	})
	queriesCachedCurrent = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "ip_api_proxy_queries_in_cache",
		Help: "The current number of unique queries in the cache currently",
	})
	cacheHits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ip_api_proxy_cache_hits_total",
		Help: "The total number of times that cache has served up a request",
	})
	successfulQueries = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ip_api_proxy_successful_queries_total",
		Help: "The total number of successfully fulfilled queries",
	})
	failedQueries = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ip_api_proxy_failed_queries_total",
		Help: "The total number of failed queries",
	})
	handlerRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ip_api_proxy_handler_requests_total",
		Help: "Total number of requests by HTTP status code",
	},
	[]string{"code"},
	)
)

func IncrementQueriesProcessed() {
	queriesProcessed.Inc()
}

func IncrementQueriesForwarded()  {
	queriesForwarded.Inc()
}

func IncrementQueriesCachedTotal()  {
	queriesCachedTotal.Inc()
}

func IncrementQueriesCachedCurrent() {
	queriesCachedCurrent.Inc()
}

func DecreaseQueriesCachedCurrent()  {
	queriesCachedCurrent.Dec()
}

func IncrementCacheHits() {
	cacheHits.Inc()
}

func IncrementSuccessfulQueries()  {
	successfulQueries.Inc()
}

func IncrementFailedQueries()  {
	failedQueries.Inc()
}

func IncrementHandlerRequests(code string)  {
	handlerRequests.With(prometheus.Labels{"code":code}).Inc()
}