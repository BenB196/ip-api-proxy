package ipAPI

import (
	"errors"
	"regexp"
	"strings"
)

var AllowedAPIFields = []string{"status","message","continent","continentCode","country","countryCode","region","regionName","city","district","zip","lat","lon","timezone","isp","org","as","asname","reverse","mobile","proxy","query"}

var AllowedLangs = []string{"en","de","es","pt-BR","fr","ja","zh-CN","ru"}

var IPDNSRegexp = regexp.MustCompile(`((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)(\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)){3}|(([a-zA-Z])|([a-zA-Z][a-zA-Z])|([a-zA-Z][0-9])|([0-9][a-zA-Z])|([a-zA-Z0-9][a-zA-Z0-9-_]{1,61}[a-zA-Z0-9]))\.([a-zA-Z]{2,6}|[a-zA-Z0-9-]{2,30}\.[a-zA-Z]{2,3}))`)

/*
ValidateFields - validates the fields string to make sure it only has valid parameters
fields - string of comma separated values
 */
func ValidateFields(fields string) (string, error) {
	fieldsSlice := strings.Split(fields,",")

	for _, field := range fieldsSlice {
		if !contains(AllowedAPIFields, field) {
			return "", errors.New("error: illegal field provided: " + field)
		}
	}

	return fields, nil
}

/*
ValidateLang - validates the lang string to make sure it is a valid lang option
lang - string with lang value
 */
func ValidateLang(lang string) (string, error) {
	if !contains(AllowedLangs,lang) {
		return "", errors.New("error: illegal lang value provided: " + lang)
	}

	return lang, nil
}

/*
contains - checks a string slice to see if it contains a string
slice - string slice which you want to check
item - string which you want to see if exists in the string slice

returns
bool - true if slice contains string, else false
 */
func contains(slice []string, item string) bool {
	for _, value := range slice {
		if value == item {
			return true
		}
	}
	return false
}