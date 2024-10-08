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
	"github.com/SENERGY-Platform/permissions-v2/pkg/client"
	"github.com/SENERGY-Platform/process-model-repository/lib/auth"
	"github.com/SENERGY-Platform/process-model-repository/lib/model"
	"github.com/beevik/etree"
	"github.com/google/uuid"
	"log"
	"net/http"
	"runtime/debug"
	"time"
)

/////////////////////////
//		api
/////////////////////////

const TIMEOUT = 10 * time.Second

func (this *Controller) ReadProcess(token auth.Token, id string, action model.AuthAction) (result model.Process, err error, errCode int) {
	access, err := this.checkBool(token, this.config.ProcessTopic, id, action)
	if err != nil {
		return result, err, http.StatusInternalServerError
	}
	if !access {
		return result, errors.New("access denied"), http.StatusForbidden
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

func (this *Controller) ListProcesses(token auth.Token, options model.ListOptions) (result []model.Process, total int64, err error, code int) {
	ids := []string{}
	//check permissions
	if options.Ids == nil {
		if token.IsAdmin() {
			ids = nil //no auth check for admins -> no id filter
		} else {
			ids, err, _ = this.perm.ListAccessibleResourceIds(token.Jwt(), this.config.ProcessTopic, client.ListOptions{}, options.Permission.ToPermission())
			if err != nil {
				return result, total, err, http.StatusInternalServerError
			}
		}
	} else {
		options.Limit = 0
		options.Offset = 0
		idMap, err, _ := this.perm.CheckMultiplePermissions(token.Jwt(), this.config.ProcessTopic, options.Ids, options.Permission.ToPermission())
		if err != nil {
			return result, total, err, http.StatusInternalServerError
		}
		for id, ok := range idMap {
			if ok {
				ids = append(ids, id)
			}
		}
	}
	options.Ids = ids
	ctx, _ := context.WithTimeout(context.Background(), TIMEOUT)
	result, total, err = this.db.ListProcesses(ctx, options)
	if err != nil {
		return result, total, err, http.StatusInternalServerError
	}
	return result, total, err, http.StatusOK
}

func (this *Controller) checkBool(token auth.Token, kind string, id string, action model.AuthAction) (allowed bool, err error) {
	if token.IsAdmin() {
		return true, nil
	}

	allowed, err, _ = this.perm.CheckPermission(token.Jwt(), kind, id, action.ToPermission())
	return allowed, err
}

func (this *Controller) PublishProcessCreate(token auth.Token, process model.Process) (result model.Process, err error, code int) {
	process.Id = uuid.NewString()
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
	process.Owner = token.GetUserId()
	err = this.producer.PublishProcessPut(process.Id, token.GetUserId(), process)
	if err != nil {
		return result, err, http.StatusInternalServerError
	}
	return process, nil, http.StatusOK
}

func (this *Controller) PublishProcessUpdate(token auth.Token, id string, process model.Process) (result model.Process, err error, code int) {
	if process.Id != id {
		return result, errors.New("path id != process.id"), http.StatusBadRequest
	}
	if process.Name == "" {
		process.Name, err = this.GetProcessModelName(process.BpmnXml)
		if err != nil {
			return result, err, http.StatusBadRequest
		}
	}
	old, err, code := this.ReadProcess(token, id, model.WRITE)
	if err != nil && err.Error() != "not found" {
		return result, err, code
	}
	if old.Owner != "" {
		process.Owner = old.Owner
	} else {
		process.Owner = token.GetUserId()
	}
	err = process.Validate()
	if err != nil {
		return result, err, http.StatusBadRequest
	}
	err = this.producer.PublishProcessPut(process.Id, token.GetUserId(), process)
	if err != nil {
		return result, err, http.StatusInternalServerError
	}
	return process, nil, http.StatusOK
}

func (this *Controller) PublishProcessPublicUpdate(token auth.Token, id string, publicCommand model.PublicCommand) (result model.Process, err error, code int) {
	process, err, code := this.ReadProcess(token, id, model.WRITE)
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

	err = this.producer.PublishProcessPut(process.Id, token.GetUserId(), process)
	if err != nil {
		return result, err, http.StatusInternalServerError
	}
	return process, nil, http.StatusOK
}

func (this *Controller) PublishProcessDelete(token auth.Token, id string) (error, int) {
	access, err := this.checkBool(token, this.config.ProcessTopic, id, model.ADMINISTRATE)
	if err != nil {
		return err, http.StatusInternalServerError
	}
	if !access {
		return errors.New("access denied"), http.StatusForbidden
	}
	err = this.producer.PublishProcessDelete(id, token.GetUserId())
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

func (this *Controller) SetProcess(owner string, process model.Process) error {
	_, err, code := this.perm.GetResource(client.InternalAdminToken, this.config.ProcessTopic, process.Id)
	if err != nil && code != http.StatusNotFound {
		return err
	}
	if code == http.StatusNotFound {
		_, err, _ = this.perm.SetPermission(client.InternalAdminToken, this.config.ProcessTopic, process.Id, client.ResourcePermissions{
			UserPermissions: map[string]client.PermissionsMap{
				owner: {
					Read:         true,
					Write:        true,
					Execute:      true,
					Administrate: true,
				},
			},
		})
		if err != nil {
			return err
		}
	}
	ctx, _ := context.WithTimeout(context.Background(), TIMEOUT)
	return this.db.SetProcess(ctx, process)
}

func (this *Controller) DeleteProcess(id string) error {
	ctx, _ := context.WithTimeout(context.Background(), TIMEOUT)
	err, _ := this.perm.RemoveResource(client.InternalAdminToken, this.config.ProcessTopic, id)
	if err != nil {
		return err
	}
	return this.db.DeleteProcess(ctx, id)
}
