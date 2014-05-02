package main

import (
	"fmt"
	"strconv"
	"github.com/naoina/kocha/util"
)

func printSettingEnv() {
	env, err := util.FindSettingEnv()
	if err != nil {
		panic(err)
	}
	fmt.Println("NOTE: You can setting your app by using following environment variables when launching an app:\n")
	for key, value := range env {
		fmt.Printf("%4s%v=%v\n", "", key, strconv.Quote(value))
	}
	fmt.Println()
}
