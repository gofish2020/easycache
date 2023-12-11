package main

import (
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gofish2020/easycache"
	"github.com/gofish2020/easycache/utils"
)

type MyData struct {
	Name  string
	Sex   int
	Score uint32
}

func main() {

	cache, err := easycache.New(easycache.TestConfig())
	if err != nil {
		return
	}

	//setRandExpiredKey(cache)
	//basic(cache)
	//testLruKey(cache)
	//testSetKey(cache)
	//testGetOrSet(cache)
	//testDelete(cache)
	testRemoveCallback(cache)

	time.Sleep(5 * time.Second)

	cache.Close()
	sigusr1 := make(chan os.Signal, 1)
	signal.Notify(sigusr1, syscall.SIGTERM)
	<-sigusr1
}

func basic(cache *easycache.EasyCache) {
	//set expire key
	err := cache.Set("easy_key_expire", &MyData{
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
}

// test expire
func setRandExpiredKey(cache *easycache.EasyCache) {
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < 20; i++ {
		key := string(utils.RandomValue(20))
		cache.Set(key, i, time.Duration(rand.Intn(10))*time.Second)
	}
	cache.Check()
}

// test lru
/*

使用 TestConfig 可以测试下面的情况
*/
func testLruKey(cache *easycache.EasyCache) {

	for i := 0; i < 4; i++ { // lru 【2 0】 【3 1】
		key := strconv.Itoa(i)
		cache.Set(key, i, 0*time.Second)
	}

	cache.Get("0") // lru 【0 2】 【3 1】
	cache.Get("1") // lru 【0 2】 【1 3】

	cache.Set("4", 4, 0*time.Second) // del 2  lru [4 0] [1 3]
	cache.Set("5", 4, 0*time.Second) // del 3  lru [4 0] [5 1]
	cache.Check()
}

// test set
func testSetKey(cache *easycache.EasyCache) {
	rand.Seed(time.Now().UnixNano())

	key := string(utils.RandomValue(20))

	// same key
	cache.Set(key, 0, 5*time.Second) // expire 5s
	cache.Set(key, 0, 0*time.Second) // expire 0s

	// same key1
	key1 := string(utils.RandomValue(20))
	cache.Set(key1, 0, 0*time.Second) // expire 0s
	cache.Set(key1, 0, 5*time.Second) // expire 5s
	cache.Check()
}

// test getorset

func testGetOrSet(cache *easycache.EasyCache) {

	key := string(utils.RandomValue(20))
	cache.GetOrSet(key, 0, 5*time.Second) // expire 5s
	time.Sleep(6 * time.Second)
	cache.GetOrSet(key, 0, 0*time.Second) //
	time.Sleep(6 * time.Second)
	fmt.Println(cache.Get(key))

	cache.Check()

}

// test del

func testDelete(cache *easycache.EasyCache) {

	key := string(utils.RandomValue(20))
	cache.Set(key, 0, 0*time.Second)
	cache.Delete(key)
	cache.Check()

	key1 := string(utils.RandomValue(20))
	cache.Set(key1, 0, 2*time.Second)
	cache.Delete(key1)
	cache.Check()
}

// test remove callback

func testRemoveCallback(cache *easycache.EasyCache) {

	key := string(utils.RandomValue(20))

	cache.Set(key, 0, 0*time.Second)
	cache.Delete(key)
	cache.Set(string(utils.RandomValue(20)), 1, 3*time.Second)
}
