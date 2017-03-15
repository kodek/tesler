package common

import (
	"encoding/json"
	"os"
)

type Configuration struct {
	TeslaAuth TeslaAuth
}

type TeslaAuth struct {
	ClientId     string
	ClientSecret string
	Username     string
	Password     string
}

func LoadConfig() Configuration {
	f, err := os.Open(os.Getenv("HOME") + "/.tesla_conf.json")
	if err != nil {
		panic(err)
	}
	decoder := json.NewDecoder(f)
	conf := Configuration{}
	err = decoder.Decode(&conf)
	if err != nil {
		panic(err)
	}
	return conf
}
