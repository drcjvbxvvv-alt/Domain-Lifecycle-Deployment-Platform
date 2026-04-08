package main

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/viper"
)

func main() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./configs")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			log.Fatalf("read config: %v", err)
		}
	}

	fmt.Fprintln(os.Stdout, "domain-platform migrate starting...")
}
