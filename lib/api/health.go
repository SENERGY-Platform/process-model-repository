package api

import (
	"bytes"
	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/julienschmidt/httprouter"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

func init() {
	endpoints = append(endpoints, HealthEndpoints)
}

const connectivityTestToken = "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJjb25uZWN0aXZpdHktdGVzdCJ9.OnihzQ7zwSq0l1Za991SpdsxkktfrdlNl-vHHpYpXQw"

func HealthEndpoints(config config.Config, control Controller, router *httprouter.Router) {
	router.POST("/health", func(writer http.ResponseWriter, request *http.Request, params httprouter.Params) {
		msg, err := ioutil.ReadAll(request.Body)
		log.Println("INFO: /health", err, string(msg))
		writer.WriteHeader(http.StatusOK)
	})

	if config.ConnectivityTest {
		go func() {
			ticker := time.NewTicker(1 * time.Minute)
			for t := range ticker.C {
				log.Println("INFO: connectivity test: " + t.String())
				client := http.Client{
					Timeout: 5 * time.Second,
				}

				req, err := http.NewRequest(
					"POST",
					"http://localhost:"+config.ServerPort+"/health",
					bytes.NewBuffer([]byte("local connection test: "+t.String())),
				)

				if err != nil {
					log.Fatal("FATAL: connection test unable to build request:", err)
				}
				req.Header.Set("Authorization", connectivityTestToken)

				resp, err := client.Do(req)
				if err != nil {
					log.Fatal("FATAL: connection test:", err)
				}
				io.ReadAll(resp.Body)
				resp.Body.Close()
			}
		}()
	}
}
