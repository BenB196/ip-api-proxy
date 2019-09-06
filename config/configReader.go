package config

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"ip-api-proxy/utils"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Cache 		Cache 		`json:"cache,omitempty"`
	APIKey		string		`json:"apiKey,omitempty"`
	Port		int			`json:"port,omitempty"`
	Prometheus	Prometheus	`json:"prometheus,omitempty"`
}

type Cache struct {
	Persist			bool	`json:"persist,omitempty"`
	CleanInterval	string	`json:"cleanInterval,omitempty"`
	WriteInterval	string	`json:"writeInterval,omitempty"`
	WriteLocation	string	`json:"writeLocation,omitempty"`
	Age				string	`json:"age,omitempty"`
}

type Prometheus struct {
	Enabled	bool	`json:"enabled,omitempty"`
	Port	int		`json:"port,omitempty"`
}

func ReadConfig(configLocation string) (Config,error) {
	var err error

	//get working directory if no location passed
	if configLocation == "" {
		configLocation, err = os.Getwd()
		configLocation = configLocation + utils.DirPath + "config.json"
		if err != nil {
			return Config{}, errors.New("error: getting working directory: " + err.Error())
		}
	} else {
		//get absolute path of config file if specified
		configLocation, err = filepath.Abs(configLocation)

		if err != nil {
			return Config{}, errors.New("error: getting absolute path of config location: " + err.Error())
		}
	}

	//open config file
	var skipConfig bool
	//init config var
	var config Config
	configFile, err := os.Open(configLocation)

	if err != nil {
		if strings.Contains(err.Error(), "The system cannot find the file specified") || strings.Contains(err.Error(), "no such file or directory") {
			skipConfig = true
			config = Config{}
		} else {
			return Config{}, errors.New("error: opening config file: " + err.Error())
		}
	}

	if !skipConfig {
		//Read config file to bytes
		fileData, err := ioutil.ReadAll(configFile)

		if err != nil {
			return Config{}, errors.New("error: reading config file: " + err.Error())
		}

		//unmarshal config with json
		err = json.Unmarshal(fileData,&config)

		if err != nil {
			return Config{}, errors.New("error: unmarshaling config file to json: " + err.Error())
		}
	}

	//validate cache
	//validate clean interval
	if config.Cache.CleanInterval != "" {
		_, err = time.ParseDuration(config.Cache.CleanInterval)

		if err != nil {
			return Config{}, errors.New("error: parsing clean interval duration: " + err.Error())
		}
	} else {
		//set to default of 30 minutes
		config.Cache.CleanInterval = "30m"
	}

	//only validate write interval and location if cache persist is true
	if config.Cache.Persist {
		//validate write interval
		if config.Cache.WriteInterval != "" {
			_, err = time.ParseDuration(config.Cache.WriteInterval)

			if err != nil {
				return Config{}, errors.New("error: parsing write interval duration: " + err.Error())
			}
		} else {
			//set to default 30 minutes
			config.Cache.WriteInterval = "30m"
		}

		//validate write location if not empty string
		if config.Cache.WriteLocation != "" {
			//make sure output location is valid
			err = utils.IsWritable(config.Cache.WriteLocation)

			if err != nil {
				return Config{}, err
			}

			//Append a / or \\ to end of path if not there
			lastChar := config.Cache.WriteLocation[len(config.Cache.WriteLocation)-1:]
			if lastChar != utils.DirPath {
				config.Cache.WriteLocation = config.Cache.WriteLocation + utils.DirPath
			}
		} else {
			//else set write location to working directory
			config.Cache.WriteLocation, err = os.Getwd()

			//return any errors
			if err != nil {
				return config, errors.New("error: unable to get working directory: " + err.Error())
			}

			//check if directory is writable
			err = utils.IsWritable(config.Cache.WriteLocation)
			//return any errors
			if err != nil {
				return config, err
			}
			//update output location to absolute path
			config.Cache.WriteLocation = config.Cache.WriteLocation + utils.DirPath
		}
	}

	//validate cache age
	if config.Cache.Age != "" {
		_, err := time.ParseDuration(config.Cache.Age)

		if err != nil {
			return Config{}, errors.New("error: parsing age duration: " + err.Error())
		}
	} else {
		//set to default 24 hours
		config.Cache.Age = "24h"
	}

	//validate port
	if config.Port == 0 {
		//set default 8080
		config.Port = 8080
	} else if config.Port < 1024 {
		return Config{}, errors.New("error: port cannot be below 1024")
	} else if config.Port > 65535 {
		return Config{}, errors.New("error: port cannot be above 65535")
	}

	return config, nil

}