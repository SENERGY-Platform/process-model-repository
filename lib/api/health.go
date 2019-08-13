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
				resp, err := client.Post("http://localhost:"+config.ServerPort+"/health", "application/json", bytes.NewBuffer([]byte("local connection test: "+t.String())))
				if err != nil {
					log.Fatal("FATAL: connection test:", err)
				}
				ioutil.ReadAll(resp.Body)
				resp.Body.Close()
			}
		}()
	}
}
