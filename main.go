package main

import (
	"container/list"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// JSON data for responding to client
type response struct {
	Action string  `json:"action"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Answer float64 `json:"answer"`
	Cached bool    `json:"cached"`
}

type cacheEntry struct {
	key    string
	answer float64
	time   time.Time

	element *list.Element
}

// Must be a pointer to cacheEntry, or the cacheEntry will be unaddressable.
// And if it's unaddressable, then the timestamp can't be updated without
// assigning a new cacheEntry to the map's key.
type cacheMap map[string]*cacheEntry

type cacheStruct struct {
	hash cacheMap  // used for quick lookups; key by question string
	list list.List // used for quick cleanup; ordered by age
}

const cacheExpireSeconds = 60

var cache *cacheStruct

func newCache() *cacheStruct {
	c := &cacheStruct{}
	c.hash = cacheMap{}
	c.list.Init()

	return c
}

func (c *cacheStruct) get(key string) (float64, bool) {
	var val float64
	item, exists := c.hash[key]

	if exists {
		val = item.answer

		now := time.Now()
		fmt.Printf("Age: %fs\n", float32(now.Sub(item.time))/float32(time.Second))

		// update timestamp
		item.time = now
		// move to the back of the list
		c.list.MoveToBack(item.element)
	}

	return val, exists
}

func (c *cacheStruct) set(key string, value float64) {
	now := time.Now()

	entry := &cacheEntry{key, value, now, nil}
	c.hash[key] = entry
	element := c.list.PushBack(entry)
	entry.element = element
}

func (c *cacheStruct) cleanup() {
	now := time.Now()
	expireTime := now.Add(time.Second * -cacheExpireSeconds)

	// Iterate through list. Remove all expired items at the front, and stop
	// when we reach the first non-expired item.
	fmt.Printf("\ncache.list (%d):\n", c.list.Len())
	var next *list.Element
	for e := c.list.Front(); e != nil; e = next {
		next = e.Next() // store Next() because we might remove this element

		value := e.Value.(*cacheEntry)
		if value.time.Before(expireTime) {
			c.list.Remove(e)
			delete(c.hash, value.key)
		} else {
			break
		}
	}
}

func getFormFloat(r *http.Request, name string) (float64, error) {
	strVal := r.FormValue(name)
	if strVal == "" {
		return 0, fmt.Errorf("%s is undefined", name)
	}

	val, err := strconv.ParseFloat(strVal, 64)
	if err != nil {
		return 0, fmt.Errorf("%s is not a number: %v", name, strVal)
	}

	return val, nil
}

func getXY(r *http.Request) (float64, float64, error) {
	x, err := getFormFloat(r, "x")
	if err != nil {
		return 0, 0, err
	}

	y, err := getFormFloat(r, "y")
	if err != nil {
		return 0, 0, err
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

	cache.cleanup()

	cacheAnswer, exists := cache.get(reqString)

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

		cache.set(reqString, answer)
	}

	data := response{
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
