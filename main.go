package main

import (
	"container/list"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// JSON data for responding to client
type Response struct {
	Action string  `json:"action"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Answer float64 `json:"answer"`
	Cached bool    `json:"cached"`
}

type CacheEntry struct {
	Key    string
	Answer float64
	Time   time.Time

	element *list.Element
}

// Must be a pointer to CacheEntry, or the CacheEntry will be unaddressable.
// And if it's unaddressable, then the timestamp can't be updated without
// assigning a new CacheEntry to the map's key.
type CacheMap map[string]*CacheEntry

type Cache struct {
	hash CacheMap  // used for quick lookups; key by question string
	list list.List // used for quick cleanup; ordered by age
}

const cacheExpireSeconds = 60

var cache *Cache

func newCache() *Cache {
	c := &Cache{}
	c.hash = CacheMap{}
	c.list.Init()

	return c
}

func (c *Cache) Get(key string) (float64, bool) {
	var val float64
	item, exists := c.hash[key]

	if exists {
		val = item.Answer

		now := time.Now()
		fmt.Printf("Age: %fs\n", float32(now.Sub(item.Time))/float32(time.Second))

		// update timestamp
		item.Time = now
		// move to the back of the list
		c.list.MoveToBack(item.element)
	}

	return val, exists
}

func (c *Cache) Set(key string, value float64) {
	now := time.Now()

	entry := &CacheEntry{key, value, now, nil}
	c.hash[key] = entry
	element := c.list.PushBack(entry)
	entry.element = element
}

func (c *Cache) Cleanup() {
	now := time.Now()
	expireTime := now.Add(time.Second * -cacheExpireSeconds)

	// Iterate through list. Remove all expired items at the front, and stop
	// when we reach the first non-expired item.
	fmt.Printf("\ncache.list (%d):\n", c.list.Len())
	var next *list.Element
	for e := c.list.Front(); e != nil; e = next {
		next = e.Next() // store Next() because we might remove this element

		value := e.Value.(*CacheEntry)
		if value.Time.Before(expireTime) {
			c.list.Remove(e)
			delete(c.hash, value.Key)
		} else {
			break
		}
	}
}

func getXY(r *http.Request) (float64, float64, error) {
	r.ParseForm()
	form_x := r.FormValue("x")
	if form_x == "" {
		return 0, 0, errors.New("x is undefined")
	}
	x, err := strconv.ParseFloat(form_x, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("x is not a number: %v", form_x)
	}

	form_y := r.FormValue("y")
	if form_y == "" {
		return 0, 0, errors.New("y is undefined")
	}
	y, err := strconv.ParseFloat(form_y, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("y is not a number: %v", form_y)
	}

	return x, y, nil
}

func doMath(w http.ResponseWriter, r *http.Request) {
	op := r.URL.Path[1:]

	x, y, err := getXY(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Make a question string. This ensures that the map will have a unique
	// and hashable key for each question. Originally, I used r.URL as the
	// key, but it would make duplicate cache entries if x and y were swapped
	// in the query string, or if extra data was added to the query.
	reqString := fmt.Sprintf("%s;%v;%v", op, x, y)

	var answer float64

	cache.Cleanup()

	cacheAnswer, exists := cache.Get(reqString)

	if exists {
		answer = cacheAnswer
	} else {

		// Note: invalid operations won't be passed to doMath
		switch op {
		case "add":
			answer = x + y
		case "subtract":
			answer = x - y
		case "multiply":
			answer = x * y
		case "divide":
			// Floating point division is not subject to divide-by-zero error
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
	cache = newCache()
	fmt.Println("Running web server on port 8080")

	// Only allow valid operations to be sent to doMath
	http.HandleFunc("/add", doMath)
	http.HandleFunc("/subtract", doMath)
	http.HandleFunc("/multiply", doMath)
	http.HandleFunc("/divide", doMath)

	err := http.ListenAndServe(":8080", nil)
	fmt.Printf("Error: %v", err)
}
