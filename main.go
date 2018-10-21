package main

import (
	"encoding/json"
	"errors"
	"fmt"
	//"html"
	"net/http"
	"strconv"
	"time"
)

type Response struct {
	Action string  `json:"action"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Answer float64 `json:"answer"`
	Cached bool    `json:"cached"`
}

type CacheEntry struct {
	Answer float64
	Time   time.Time
}

// Must be a pointer to CacheEntry, or the CacheEntry will be unaddressable.
// And if it's unaddressable, then the timestamp can't be updated without
// assigning a new CacheEntry to the map's key.
type CacheMap map[string]*CacheEntry

type Cache struct {
	hash CacheMap
	//list CacheList
}

func (c *Cache) Init() {
	c.hash = CacheMap{}
}

func (c *Cache) Get(key string) (val float64, exists bool) {
	item, exists := c.hash[key]

	if exists {
		val = item.Answer

		now := time.Now()
		fmt.Printf("Age: %fs\n", float32(now.Sub(item.Time))/float32(time.Second))
		item.Time = now
	}

	return
}

func (c *Cache) Set(key string, value float64) {
	now := time.Now()

	c.hash[key] = &CacheEntry{value, now}
}

func (c *Cache) Delete(key string) {
	delete(c.hash, key)
}

func (c *Cache) Cleanup() {
	now := time.Now()
	expireTime := now.Add(time.Second * -60)

	// TODO: use a linked list to make c cleanup an O(1) operation
	fmt.Println("\ncache:")
	for key, value := range c.hash {
		deleteIt := false
		if value.Time.Before(expireTime) {
			deleteIt = true
			c.Delete(key)
		}

		fmt.Println(key, value, deleteIt)
	}
}

var cache Cache

func getXY(r *http.Request) (float64, float64, error) {
	r.ParseForm()
	form_x, ok := r.Form["x"]
	if !ok {
		return 0, 0, errors.New("x is undefined")
	}
	x, err := strconv.ParseFloat(form_x[0], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("x is not an integer: %v", form_x[0])
	}

	form_y, ok := r.Form["y"]
	if !ok {
		return 0, 0, errors.New("y is undefined")
	}
	y, err := strconv.ParseFloat(form_y[0], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("y is not an integer: %v", form_y[0])
	}

	return x, y, nil
}

func doMath(w http.ResponseWriter, r *http.Request) {
	op := r.URL.Path[1:]

	x, y, err := getXY(r)
	if err != nil {
		http.Error(w, err.Error(), 500)
		fmt.Printf("Error: %v\n", err)
		return
	}

	reqString := fmt.Sprintf("%s;%v;%v", op, x, y)

	var answer float64

	cache.Cleanup()

	cacheAnswer, exists := cache.Get(reqString)

	if exists {
		answer = cacheAnswer
	} else {

		switch op {
		case "add":
			answer = x + y
		case "subtract":
			answer = x - y
		case "multiply":
			answer = x * y
		case "divide":
			answer = x / y
		}

		cache.Set(reqString, answer)
	}

	data := Response{
		Action: op,
		X:      x,
		Y:      y,
		Answer: answer,
		Cached: exists,
	}

	ret, err := json.Marshal(data)
	if err != nil {
		http.Error(w, err.Error(), 500)
		fmt.Printf("Error: %v\n", err)
	}

	w.Header().Set("Content-Type", "application/json")

	fmt.Fprintf(w, "%s", ret)
}

func main() {
	cache = Cache{}
	cache.Init()
	fmt.Println("Running web server on port 8080")

	http.HandleFunc("/add", doMath)
	http.HandleFunc("/subtract", doMath)
	http.HandleFunc("/multiply", doMath)
	http.HandleFunc("/divide", doMath)

	err := http.ListenAndServe(":8080", nil)
	fmt.Printf("Error: %v", err)
}
