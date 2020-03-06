package main

type EcsLocation struct {
	Status 			string		`json:"status,omitempty"`
	Message			string		`json:"message,omitempty"`
	Continent		string		`json:"continent,omitempty"`
	ContinentCode	string		`json:"continent_iso_code,omitempty"`
	Country			string		`json:"country,omitempty"`
	CountryCode		string		`json:"country_iso_code,omitempty"`
	Region			string		`json:"region_iso_code,omitempty"`
	RegionName		string		`json:"region_name,omitempty"`
	City			string		`json:"city_name,omitempty"`
	District		string		`json:"district,omitempty"`
	ZIP				string		`json:"postal_code,omitempty"`
	Lat				*float32	`json:"lat,omitempty"`
	Lon				*float32	`json:"lon,omitempty"`
	Timezone		string		`json:"timezone,omitempty"`
	Currency		string		`json:"currency,omitempty"`
	ISP				string		`json:"isp,omitempty"`
	Org				string		`json:"org,omitempty"`
	AS				string		`json:"as,omitempty"`
	ASName			string		`json:"asname,omitempty"`
	Reverse			string		`json:"reverse,omitempty"`
	Mobile			*bool		`json:"mobile,omitempty"`
	Proxy			*bool		`json:"proxy,omitempty"`
	Hosting			*bool		`json:"hosting,omitempty"`
	Query			string		`json:"query,omitempty"`
}