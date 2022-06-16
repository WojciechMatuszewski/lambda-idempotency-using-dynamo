package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"
)

const API_URL = "xxx"

func main() {
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(iteration int) {
			defer wg.Done()
			fmt.Printf("[%d iteration] starting \n", iteration)
			out, err := http.Post(
				API_URL,
				"application/json",
				bytes.NewBuffer([]byte(`{"name": "test"}`)),
			)
			if err != nil {
				fmt.Printf("[%d iteration] error %s \n", iteration, err.Error())
			}

			buf, err := io.ReadAll(out.Body)
			if err != nil {
				fmt.Printf("[%d iteration] error %s \n", iteration, err.Error())
			}

			fmt.Printf("[%d iteration] response %s \n", iteration, string(buf))
		}(i)
	}

	wg.Wait()
}
