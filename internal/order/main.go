package main

import (
	"github.com/jiahuipaung/gorder/common/config"
	"github.com/spf13/viper"
	"log"
)

func init() {
	if err := config.NewViperConfig(); err != nil {
		log.Fatal(err)
	}
}

func main() {
	log.Print("%v", viper.Get("order"))
}
