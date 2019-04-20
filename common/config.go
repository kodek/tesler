package common

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

type Configuration struct {
	Recorder Recorder
}

type Recorder struct {
	Port           int
	TeslaAuth      TeslaAuth
	Cars           []Car
	InfluxDbConfig InfluxDbConfig
	Pushover       PushoverConfig
}
type Car struct {
	Monitor bool
	Vin     string
}

type TeslaAuth struct {
	ClientId     string
	ClientSecret string
	Username     string
	Password     string
}

type InfluxDbConfig struct {
	Address  string
	Username string
	Password string
	Database string
}

type PushoverConfig struct {
	Token string
	User  string
}

var configPath = flag.String("config", "", "The path to the config file")

func LoadConfig() Configuration {
	var path string
	if *configPath == "" {
		path = os.Getenv("HOME") + "/.tesla_conf.json"
	} else {
		path = *configPath
	}

	f, err := os.Open(path)
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

func (c *Configuration) WriteRedacted(w io.Writer) {
	fmt.Fprintf(w, "Config redaction not implemented.")
}
