/*
 * Copyright 2019 InfAI (CC SES)
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package api

import (
	"context"
	"github.com/SENERGY-Platform/process-model-repository/lib/api/util"
	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/SENERGY-Platform/process-model-repository/lib/contextwg"
	"github.com/julienschmidt/httprouter"
	"log"
	"net/http"
	"reflect"
	"runtime"
	"time"
)

var endpoints = []func(config config.Config, control Controller, router *httprouter.Router){}

func Start(ctx context.Context, config config.Config, control Controller) {
	log.Println("start api")
	router := httprouter.New()
	for _, e := range endpoints {
		log.Println("add endpoints: " + runtime.FuncForPC(reflect.ValueOf(e).Pointer()).Name())
		e(config, control, router)
	}
	log.Println("add logging and cors")
	corsHandler := util.NewCors(router)
	logger := util.NewLogger(corsHandler, config.LogLevel)
	server := &http.Server{Addr: ":" + config.ServerPort, Handler: logger, WriteTimeout: 10 * time.Second, ReadTimeout: 2 * time.Second, ReadHeaderTimeout: 2 * time.Second}
	go func() {
		log.Println("Listening on ", server.Addr)
		if err := server.ListenAndServe(); err != nil {
			if err != http.ErrServerClosed {
				log.Println("ERROR: api server error", err)
				log.Fatal(err)
			} else {
				log.Println("closing api server")
			}
		}
	}()
	contextwg.Add(ctx, 1)
	go func() {
		<-ctx.Done()
		log.Println("DEBUG: api shutdown", server.Shutdown(context.Background()))
		contextwg.Done(ctx)
	}()
	return
}
