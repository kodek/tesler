package car

import (
	"github.com/kodek/tesla"
	"github.com/kodek/tesler/common"
)

// Creates a tesla.Client from the server's configuration.
func NewTeslaClientFromConfig(conf common.Configuration) (*tesla.Client, error) {
	return tesla.NewClient(getTeslaAuth(conf))
}

func getTeslaAuth(conf common.Configuration) *tesla.Auth {
	teslaConf := conf.Recorder.TeslaAuth
	return &tesla.Auth{
		ClientID:     teslaConf.ClientId,
		ClientSecret: teslaConf.ClientSecret,
		Email:        teslaConf.Username,
		Password:     teslaConf.Password,
	}
}
