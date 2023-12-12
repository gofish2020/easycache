package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofish2020/easycache"
)

type MyData struct {
	Name  string
	Sex   int
	Score uint32
}

func main() {

	conf := easycache.DefaultConfig()
	conf.Verbose = true
	conf.Shards = 4
	cache, err := easycache.New(conf)
	if err != nil {
		return
	}

	//set expire key
	err = cache.Set("easy_key_expire", &MyData{
		Name:  "小红",
		Sex:   1,
		Score: 100,
	}, 5*time.Second)

	if err != nil {
		return
	}

	//set persist key
	err = cache.Set("easy_key_persist", &MyData{
		Name:  "小明",
		Sex:   2,
		Score: 99,
	}, 0)
	if err != nil {
		return
	}

	// get expire key
	val, err := cache.Get("easy_key_expire")
	if err == nil {
		fmt.Println(val.(*MyData))
	}

	// get persisit key
	val, err = cache.Get("easy_key_persist")
	if err == nil {
		fmt.Println(val.(*MyData))
	}

	// sleep
	time.Sleep(6 * time.Second)

	//get expire key
	val1, err := cache.Get("easy_key_expire")
	if err == nil {
		fmt.Println(val1.(*MyData))
	}

	// get persisit key
	val1, err = cache.Get("easy_key_persist")
	if err == nil {
		fmt.Println(val1.(*MyData))
	}
	// close cache
	cache.Close()

	//signal block
	sigusr1 := make(chan os.Signal, 1)
	signal.Notify(sigusr1, syscall.SIGTERM)
	<-sigusr1
}
