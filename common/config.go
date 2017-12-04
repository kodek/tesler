package common

import (
	"encoding/json"
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

func (c *Configuration) WriteRedacted(w io.Writer) {
	fmt.Fprintf(w, "Config redaction not implemented.")
}
