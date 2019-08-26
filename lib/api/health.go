package api

import (
	"bytes"
	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	jwt_http_router "github.com/SmartEnergyPlatform/jwt-http-router"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

func init() {
	endpoints = append(endpoints, HealthEndpoints)
}

const connectivityTestToken = "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJjb25uZWN0aXZpdHktdGVzdCJ9.OnihzQ7zwSq0l1Za991SpdsxkktfrdlNl-vHHpYpXQw"

func HealthEndpoints(config config.Config, control Controller, router *jwt_http_router.Router) {
	router.POST("/health", func(writer http.ResponseWriter, request *http.Request, params jwt_http_router.Params, jwt jwt_http_router.Jwt) {
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
				ioutil.ReadAll(resp.Body)
				resp.Body.Close()
			}
		}()
	}
}
