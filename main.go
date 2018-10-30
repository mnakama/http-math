package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
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
}

// Must be a pointer to cacheEntry, or the cacheEntry will be unaddressable.
// And if it's unaddressable, then the timestamp can't be updated without
// assigning a new cacheEntry to the map's key.
type cacheMap map[string]*cacheEntry

type cacheStruct struct {
	hash  cacheMap // used for quick lookups; key by question string
	mutex sync.RWMutex
}

const cacheExpireSeconds = 60
const cacheCleanupInterval = 10

var cache *cacheStruct

func newCache() *cacheStruct {
	c := &cacheStruct{}
	c.hash = cacheMap{}

	go c.cleaner()

	return c
}

func (c *cacheStruct) get(key string) (float64, bool) {
	var val float64

	c.mutex.RLock()
	defer c.mutex.RUnlock()
	item, exists := c.hash[key]

	if exists {
		val = item.answer

		now := time.Now()
		expireTime := now.Add(time.Second * -cacheExpireSeconds)

		log.Printf("Age: %fs\n", float32(now.Sub(item.time))/float32(time.Second))

		if item.time.Before(expireTime) {
			// expired

			// We do not delete it from the cache now, because that would require
			// a write lock, which would delay the return of this function and
			// block all concurrent read access to the cache. Let the periodic
			// cleaner do it.
			return 0, false
		}

		// not expired; update timestamp
		item.time = now
	}

	return val, exists
}

func (c *cacheStruct) set(key string, value float64) {
	now := time.Now()

	entry := &cacheEntry{key, value, now}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.hash[key] = entry
}

func (c *cacheStruct) removeKeys(expList []string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for _, key := range(expList) {
		delete(c.hash, key)
	}
}

func (c *cacheStruct) cleanup() {
	now := time.Now()
	expireTime := now.Add(time.Second * -cacheExpireSeconds)

	// list of things to delete
	expList := make([]string, 5)

	// only obtain a RLock for now
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	log.Printf("Cache size: %d\n", len(c.hash))
	for key, value := range c.hash {
		if value.time.Before(expireTime) {
			log.Printf("Expired: %v\n", key)
			expList = append(expList, key)
		}
	}

	if len(expList) > 1 {
		// do actual cleanup in a separate goroutine while holding a write Lock.
		go c.removeKeys(expList)
	}
}

// runs in a separate goroutine
func (c *cacheStruct) cleaner() {
	for {
		time.Sleep(time.Second * cacheCleanupInterval)
		c.cleanup()
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

func getAnswer(op string, x float64, y float64) (float64, bool, error) {
	// Make a question string. This ensures that the map will have a unique
	// and hashable key for each question. Originally, I used r.URL as the
	// key, but it would make duplicate cache entries if x and y were swapped
	// in the query string, or if extra data was added to the query.
	reqString := fmt.Sprintf("%s;%v;%v", op, x, y)

	var answer float64

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
			// Floating point division is not subject to divide-by-zero error,
			// but JSON cannot handle Inf, so we check here to provide a nicer
			// error message.
			if y == 0 {
				return 0, false, errors.New("Cannot divide by zero")
			}

			answer = x / y
		default:
			return 0, false, fmt.Errorf("Invalid operation: %s", op)
		}

		cache.set(reqString, answer)
	}

	return answer, exists, nil
}

func doMath(w http.ResponseWriter, r *http.Request) {
	op := r.URL.Path[1:]

	if op == "" {
		fmt.Fprintln(w, "Usage: curl http://localhost:8080/{OP}?x={X}&y={Y}\n"+
			"\n"+
			"OP: operation (add, subtract, multiply, divide\n"+
			"X, Y: parameters")

		return
	}

	x, y, err := getXY(r)
	if err != nil {
		httpFail(w, err)
		return
	}

	answer, cached, err := getAnswer(op, x, y)
	if err != nil {
		httpFail(w, err)
		return
	}

	data := response{
		Action: op,
		X:      x,
		Y:      y,
		Answer: answer,
		Cached: cached,
	}

	ret, err := json.Marshal(data)
	if err != nil {
		httpFail(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	fmt.Fprintf(w, "%s", ret)
}

func httpFail(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
	log.Printf("Error: %v\n", err)
}

func main() {
	cache = newCache()
	log.Println("Running web server on port 8080")

	// Only allow valid operations to be sent to doMath
	http.HandleFunc("/", doMath)

	err := http.ListenAndServe(":8080", nil)
	log.Printf("Error: %v", err)
}
