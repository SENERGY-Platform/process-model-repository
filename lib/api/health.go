package api

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/julienschmidt/httprouter"
)

func init() {
	endpoints = append(endpoints, HealthEndpoints)
}

const connectivityTestToken = "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJjb25uZWN0aXZpdHktdGVzdCJ9.OnihzQ7zwSq0l1Za991SpdsxkktfrdlNl-vHHpYpXQw"

func HealthEndpoints(config config.Config, control Controller, router *httprouter.Router) {
	router.POST("/health", func(writer http.ResponseWriter, request *http.Request, params httprouter.Params) {
		msg, _ := io.ReadAll(request.Body)
		config.GetLogger().Info("health check", "message", string(msg))
		writer.WriteHeader(http.StatusOK)
	})

	if config.ConnectivityTest {
		go func() {
			ticker := time.NewTicker(1 * time.Minute)
			for t := range ticker.C {
				config.GetLogger().Info("connectivity test", "time", t.String())
				client := http.Client{
					Timeout: 5 * time.Second,
				}

				req, err := http.NewRequest(
					"POST",
					"http://localhost:"+config.ServerPort+"/health",
					bytes.NewBuffer([]byte("local connection test: "+t.String())),
				)

				if err != nil {
					config.GetLogger().Error("FATAL: connection test unable to build request", "error", err)
					log.Fatal("FATAL: connection test unable to build request:", err)
				}
				req.Header.Set("Authorization", connectivityTestToken)

				resp, err := client.Do(req)
				if err != nil {
					config.GetLogger().Error("FATAL: connection test", "error", err)
					log.Fatal("FATAL: connection test:", err)
				}
				io.ReadAll(resp.Body)
				resp.Body.Close()
			}
		}()
	}
}
