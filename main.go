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

package main

import (
	"context"
	"flag"
	"github.com/SENERGY-Platform/process-model-repository/lib"
	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/SENERGY-Platform/process-model-repository/lib/contextwg"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

func main() {
	configLocation := flag.String("config", "config.json", "configuration file")
	flag.Parse()

	conf, err := config.Load(*configLocation)
	if err != nil {
		log.Fatal("ERROR: unable to load config", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}
	ctx = contextwg.WithWaitGroup(ctx, wg)

	err = lib.Start(ctx, conf)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		shutdown := make(chan os.Signal, 1)
		signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
		sig := <-shutdown
		log.Println("received shutdown signal", sig)
		cancel()
	}()

	<-ctx.Done() //waiting for context end; may happen by shutdown signal
	wg.Wait()
}
