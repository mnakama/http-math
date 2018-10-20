package main

import (
	"encoding/json"
	"errors"
	"fmt"
	//"html"
	"net/http"
	"strconv"
)

type Response struct {
	Action string  `json:"action"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Answer float64 `json:"answer"`
	Cached bool    `json:"cached"`
}

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

	var answer float64

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

	data := Response{
		Action: op,
		X:      x,
		Y:      y,
		Answer: answer,
		Cached: false,
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
	fmt.Println("Running web server on port 8080")

	http.HandleFunc("/add", doMath)
	http.HandleFunc("/subtract", doMath)
	http.HandleFunc("/multiply", doMath)
	http.HandleFunc("/divide", doMath)

	err := http.ListenAndServe(":8080", nil)
	fmt.Printf("Error: %v", err)
}
