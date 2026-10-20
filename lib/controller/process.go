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
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/SENERGY-Platform/permissions-v2/pkg/client"
	"github.com/SENERGY-Platform/process-model-repository/lib/auth"
	"github.com/SENERGY-Platform/process-model-repository/lib/model"
	"github.com/beevik/etree"
	"github.com/google/uuid"
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

func (this *Controller) CreateProcess(token auth.Token, process model.Process) (result model.Process, err error, code int) {
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
	process.LastUpdatedUnix = time.Now().Unix()
	err = this.SetProcess(token.GetUserId(), process)
	if err != nil {
		ctx, _ := context.WithTimeout(context.Background(), TIMEOUT)
		this.db.DeleteProcess(ctx, process.Id)
		return result, err, http.StatusInternalServerError
	}
	return process, nil, http.StatusOK
}

func (this *Controller) UpdateProcess(token auth.Token, id string, process model.Process) (result model.Process, err error, code int) {
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
	process.LastUpdatedUnix = time.Now().Unix()
	err = process.Validate()
	if err != nil {
		return result, err, http.StatusBadRequest
	}
	err = this.SetProcess(token.GetUserId(), process)
	if err != nil {
		return result, err, http.StatusInternalServerError
	}
	return process, nil, http.StatusOK
}

func (this *Controller) UpdateProcessPublic(token auth.Token, id string, publicCommand model.PublicCommand) (result model.Process, err error, code int) {
	process, err, code := this.ReadProcess(token, id, model.WRITE)
	if err != nil {
		return result, err, code
	}
	process.Publish = publicCommand.Publish
	process.PublishDate = time.Now().String()
	process.LastUpdatedUnix = time.Now().Unix()
	if process.Publish {
		process.Description = publicCommand.Description
	} else {
		process.Description = ""
	}

	err = this.SetProcess(token.GetUserId(), process)
	if err != nil {
		return result, err, http.StatusInternalServerError
	}
	return process, nil, http.StatusOK
}

var fullRights = client.PermissionsMap{Read: true, Write: true, Execute: true, Administrate: true}

// SetProcessOwner moves the process model to owner, for administrators only. Owner first gets full
// rights in permissions-v2, every other entry stays as stored; then the document's owner changes.
// If the document write fails, owner merely holds extra rights and a repeat completes the move.
// last_updated_unix stays: the smart-service-repository takes a newer one as a content change and
// marks every release using the model as changed. Permissions are read and written back without
// a version check, so a permission edit made in between is lost. An UpdateProcess that read the
// document before the owner write puts the previous owner back; a repeat of the move fixes that.
func (this *Controller) SetProcessOwner(token auth.Token, id string, owner string) (result model.Process, err error, code int) {
	if !token.IsAdmin() {
		return result, errors.New("access denied"), http.StatusForbidden
	}
	if owner == "" || strings.TrimSpace(owner) != owner {
		return result, errors.New("owner must be a user id without surrounding whitespace"), http.StatusBadRequest
	}
	ctx, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()
	process, exists, err := this.db.ReadProcess(ctx, id)
	if err != nil {
		return result, err, http.StatusInternalServerError
	}
	if !exists {
		return result, errors.New("not found"), http.StatusNotFound
	}

	resource, err, _ := this.perm.GetResource(client.InternalAdminToken, this.config.ProcessTopic, id)
	if err != nil {
		// Also when permissions-v2 knows no entry: the cleanup deletes such a document, or it is
		// being created right now and an entry written here would keep its creator out.
		return result, err, http.StatusInternalServerError
	}
	permissions := resource.ResourcePermissions
	if permissions.UserPermissions[owner] != fullRights {
		if permissions.UserPermissions == nil {
			permissions.UserPermissions = map[string]client.PermissionsMap{}
		}
		permissions.UserPermissions[owner] = fullRights
		_, err, _ = this.perm.SetPermission(client.InternalAdminToken, this.config.ProcessTopic, id, permissions)
		if err != nil {
			return result, err, http.StatusInternalServerError
		}
	}

	if process.Owner != owner {
		found, err := this.db.SetProcessOwner(ctx, id, owner)
		if err != nil {
			return result, err, http.StatusInternalServerError
		}
		if !found {
			return result, errors.New("not found"), http.StatusNotFound
		}
	}
	result, exists, err = this.db.ReadProcess(ctx, id)
	if err != nil {
		return result, err, http.StatusInternalServerError
	}
	if !exists {
		return result, errors.New("not found"), http.StatusNotFound
	}
	return result, nil, http.StatusOK
}

// DeleteProcess refuses with *model.InUseError (409) while smart services use the process model.
// Force deletes anyway and is for administrators only. When the smart-service-repository cannot
// answer, nothing is deleted (502), also with force. Check and delete are not atomic: a release
// created in between is not seen.
func (this *Controller) DeleteProcess(token auth.Token, id string, force bool) (error, int) {
	if force && !token.IsAdmin() {
		return errors.New("force needs the admin role"), http.StatusForbidden
	}
	access, err := this.checkBool(token, this.config.ProcessTopic, id, model.ADMINISTRATE)
	if err != nil {
		return err, http.StatusInternalServerError
	}
	if !access {
		return errors.New("access denied"), http.StatusForbidden
	}
	ctx, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()
	usage, err := this.usage.Usage(ctx, id, token.Jwt())
	if err != nil {
		// The detail may hold internals, so only the static sentinel reaches the client.
		slog.Warn("could not ask the smart-service-repository for the usage of a process model", "process_model_id", id, "error", err)
		return model.ErrUsageUnavailable, http.StatusBadGateway
	}
	used := usage.Releases > 0
	if used && !force {
		return &model.InUseError{Usage: usage}, http.StatusConflict
	}
	err, code := this.deleteProcess(id)
	if err != nil {
		return err, code
	}
	if used {
		slog.Warn("deleted a process model that smart services still use",
			"process_model_id", id, "user_id", token.GetUserId(), "releases", usage.Releases, "instances", usage.Instances)
	}
	return nil, http.StatusOK
}

func (this *Controller) deleteProcess(id string) (error, int) {
	ctx, _ := context.WithTimeout(context.Background(), TIMEOUT)
	err, _ := this.perm.RemoveResource(client.InternalAdminToken, this.config.ProcessTopic, id)
	if err != nil {
		return err, http.StatusInternalServerError
	}
	err = this.db.DeleteProcess(ctx, id)
	if err != nil {
		return err, http.StatusInternalServerError
	}
	return nil, http.StatusOK
}

func (this *Controller) GetProcessModelName(bpmn string) (name string, err error) {
	defer func() {
		if r := recover(); r != nil && err == nil {
			slog.Error("recovered from panic", "error", r, "stack", debug.Stack())
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

func (this *Controller) SetProcess(owner string, process model.Process) error {
	ctx, _ := context.WithTimeout(context.Background(), TIMEOUT)
	if process.LastUpdatedUnix == 0 {
		process.LastUpdatedUnix = time.Now().Unix()
	}
	err := this.db.SetProcess(ctx, process)
	if err != nil {
		return err
	}
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
	return nil
}
