package main

import (
	"container/list"
	"encoding/json"
	"errors"
	"fmt"
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

	element *list.Element
}

// Must be a pointer to cacheEntry, or the cacheEntry will be unaddressable.
// And if it's unaddressable, then the timestamp can't be updated without
// assigning a new cacheEntry to the map's key.
type cacheMap map[string]*cacheEntry

type cacheStruct struct {
	hash  cacheMap  // used for quick lookups; key by question string
	list  list.List // used for quick cleanup; ordered by age
	mutex sync.Mutex
}

const cacheExpireSeconds = 60

var cache *cacheStruct

func newCache() *cacheStruct {
	c := &cacheStruct{}
	c.hash = cacheMap{}
	c.list.Init()

	return c
}

func (c *cacheStruct) lock() {
	c.mutex.Lock()
}

func (c *cacheStruct) unlock() {
	c.mutex.Unlock()
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

// I split this into its own function to reduce the time that the mutex lock
// is held, since defer only executes when we leave the function. Normally,
// I would use a with: block or a try/finally in python.
func getAnswer(op string, x float64, y float64) (float64, bool, error) {
	// Make a question string. This ensures that the map will have a unique
	// and hashable key for each question. Originally, I used r.URL as the
	// key, but it would make duplicate cache entries if x and y were swapped
	// in the query string, or if extra data was added to the query.
	reqString := fmt.Sprintf("%s;%v;%v", op, x, y)

	var answer float64

	cache.lock()
	defer cache.unlock()

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
	fmt.Printf("Error: %v\n", err)
}

func main() {
	cache = newCache()
	fmt.Println("Running web server on port 8080")

	// Only allow valid operations to be sent to doMath
	http.HandleFunc("/", doMath)

	err := http.ListenAndServe(":8080", nil)
	fmt.Printf("Error: %v", err)
}
