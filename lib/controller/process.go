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

package controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/SENERGY-Platform/process-model-repository/lib/model"
	jwt_http_router "github.com/SmartEnergyPlatform/jwt-http-router"
	"github.com/beevik/etree"
	uuid "github.com/satori/go.uuid"
	"log"
	"net/http"
	"runtime/debug"
	"time"
)

/////////////////////////
//		api
/////////////////////////

const TIMEOUT = 10 * time.Second

func (this *Controller) ReadProcess(jwt jwt_http_router.Jwt, id string, action model.AuthAction) (result model.Process, err error, errCode int) {
	access, err := this.security.CheckBool(jwt, this.config.ProcessTopic, id, action)
	if err != nil {
		return result, err, http.StatusInternalServerError
	}
	if !access {
		return result, err, http.StatusForbidden
	}
	ctx, _ := context.WithTimeout(context.Background(), TIMEOUT)
	result, exists, err := this.db.ReadProcess(ctx, id)
	if err != nil {
		return result, err, http.StatusInternalServerError
	}
	if !exists {
		return result, errors.New("not found"), http.StatusNotFound
	}
	return result, nil, http.StatusOK
}

func (this *Controller) ReadAllPublicProcess() (result []model.Process, err error, code int) {
	ctx, _ := context.WithTimeout(context.Background(), TIMEOUT)
	result, err = this.db.ReadAllPublicProcesses(ctx)
	if err != nil {
		return result, err, http.StatusInternalServerError
	}
	return result, nil, http.StatusOK
}

func (this *Controller) PublishProcessCreate(jwt jwt_http_router.Jwt, process model.Process) (result model.Process, err error, code int) {
	process.Id = uuid.NewV4().String()
	if process.Name == "" {
		process.Name, err = this.GetProcessModelName(process.BpmnXml)
		if err != nil {
			return result, err, http.StatusBadRequest
		}
	}
	err = process.Validate()
	if err != nil {
		return result, err, http.StatusBadRequest
	}
	process.Owner = jwt.UserId
	err = this.producer.PublishProcessPut(process.Id, jwt.UserId, process)
	if err != nil {
		return result, err, http.StatusInternalServerError
	}
	return process, nil, http.StatusOK
}

func (this *Controller) PublishProcessUpdate(jwt jwt_http_router.Jwt, id string, process model.Process) (result model.Process, err error, code int) {
	if process.Id != id {
		return result, errors.New("path id != process.id"), http.StatusBadRequest
	}
	if process.Name == "" {
		process.Name, err = this.GetProcessModelName(process.BpmnXml)
		if err != nil {
			return result, err, http.StatusBadRequest
		}
	}
	old, err, code := this.ReadProcess(jwt, id, model.WRITE)
	if err != nil && err.Error() != "not found" {
		return result, err, code
	}
	if old.Owner != "" {
		process.Owner = old.Owner
	} else {
		process.Owner = jwt.UserId
	}
	err = process.Validate()
	if err != nil {
		return result, err, http.StatusBadRequest
	}
	err = this.producer.PublishProcessPut(process.Id, jwt.UserId, process)
	if err != nil {
		return result, err, http.StatusInternalServerError
	}
	return process, nil, http.StatusOK
}

func (this *Controller) PublishProcessPublicUpdate(jwt jwt_http_router.Jwt, id string, publicCommand model.PublicCommand) (result model.Process, err error, code int) {
	process, err, code := this.ReadProcess(jwt, id, model.WRITE)
	if err != nil {
		return result, err, code
	}

	process.Publish = publicCommand.Publish
	process.PublishDate = time.Now().String()
	if process.Publish {
		process.Description = publicCommand.Description
	} else {
		process.Description = ""
	}

	err = this.producer.PublishProcessPut(process.Id, jwt.UserId, process)
	if err != nil {
		return result, err, http.StatusInternalServerError
	}
	return process, nil, http.StatusOK
}

func (this *Controller) PublishProcessDelete(jwt jwt_http_router.Jwt, id string) (error, int) {
	access, err := this.security.CheckBool(jwt, this.config.ProcessTopic, id, model.ADMINISTRATE)
	if err != nil {
		return err, http.StatusInternalServerError
	}
	if !access {
		return err, http.StatusForbidden
	}
	err = this.producer.PublishProcessDelete(id, jwt.UserId)
	if err != nil {
		return err, http.StatusInternalServerError
	}
	return nil, http.StatusOK
}

func (this *Controller) GetProcessModelName(bpmn string) (name string, err error) {
	defer func() {
		if r := recover(); r != nil && err == nil {
			log.Printf("%s: %s", r, debug.Stack())
			err = errors.New(fmt.Sprint("Recovered Error: ", r))
		}
	}()
	doc := etree.NewDocument()
	err = doc.ReadFromString(bpmn)
	if err != nil {
		return "", err
	}

	if len(doc.FindElements("//bpmn:collaboration")) > 0 {
		name = doc.FindElement("//bpmn:collaboration").SelectAttrValue("id", "process-name")
	} else {
		name = doc.FindElement("//bpmn:process").SelectAttrValue("id", "process-name")
	}
	return name, nil
}

/////////////////////////
//		source
/////////////////////////

func (this *Controller) SetProcess(process model.Process) error {
	ctx, _ := context.WithTimeout(context.Background(), TIMEOUT)
	return this.db.SetProcess(ctx, process)
}

func (this *Controller) DeleteProcess(id string) error {
	ctx, _ := context.WithTimeout(context.Background(), TIMEOUT)
	return this.db.DeleteProcess(ctx, id)
}
