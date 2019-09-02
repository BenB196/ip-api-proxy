package ipAPI

import (
	"errors"
	"regexp"
	"strings"
)

var AllowedAPIFields = []string{"status","message","continent","continentCode","country","countryCode","region","regionName","city","district","zip","lat","lon","timezone","isp","org","as","asname","reverse","mobile","proxy","query"}

var AllowedLangs = []string{"en","de","es","pt-BR","fr","ja","zh-CN","ru"}

var IPDNSRegexp = regexp.MustCompile(`((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)(\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)){3}|(([a-zA-Z])|([a-zA-Z][a-zA-Z])|([a-zA-Z][0-9])|([0-9][a-zA-Z])|([a-zA-Z0-9][a-zA-Z0-9-_]{1,61}[a-zA-Z0-9]))\.([a-zA-Z]{2,6}|[a-zA-Z0-9-]{2,30}\.[a-zA-Z]{2,3}))`)

func ValidateFields(fields string) (string, error) {
	fieldsSlice := strings.Split(fields,",")

	for _, field := range fieldsSlice {
		if !contains(AllowedAPIFields, field) {
			return "", errors.New("error: illegal field provided: " + field)
		}
	}

	return fields, nil
}

func ValidateLang(lang string) (string, error) {
	if !contains(AllowedLangs,lang) {
		return "", errors.New("error: illegal lang value provided: " + lang)
	}

	return lang, nil
}

func contains(slice []string, item string) bool {
	for _, value := range slice {
		if value == item {
			return true
		}
	}
	return false
}