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
	"github.com/SENERGY-Platform/process-model-repository/lib/api/util"
	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/SmartEnergyPlatform/jwt-http-router"
	"log"
	"net/http"
	"reflect"
	"runtime"
)

var endpoints = []func(config config.Config, control Controller, router *jwt_http_router.Router){}

func Start(config config.Config, control Controller) (err error) {
	log.Println("start api")
	router := jwt_http_router.New(jwt_http_router.JwtConfig{PubRsa: config.JwtPubRsa, ForceAuth: config.ForceAuth, ForceUser: config.ForceUser})
	log.Println("add heart beat endpoint")
	router.GET("/", func(writer http.ResponseWriter, request *http.Request, params jwt_http_router.Params, jwt jwt_http_router.Jwt) {
		writer.WriteHeader(http.StatusOK)
	})
	for _, e := range endpoints {
		log.Println("add endpoints: " + runtime.FuncForPC(reflect.ValueOf(e).Pointer()).Name())
		e(config, control, router)
	}
	log.Println("add logging and cors")
	corsHandler := util.NewCors(router)
	logger := util.NewLogger(corsHandler, config.LogLevel)
	log.Println("listen on port", config.ServerPort)
	go func() { log.Println(http.ListenAndServe(":"+config.ServerPort, logger)) }()
	return nil
}
