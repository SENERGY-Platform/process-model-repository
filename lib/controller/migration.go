/*
 * Copyright 2024 InfAI (CC SES)
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
	permsearch "github.com/SENERGY-Platform/permission-search/lib/client"
	permsearchmodel "github.com/SENERGY-Platform/permission-search/lib/model"
	"github.com/SENERGY-Platform/permissions-v2/pkg/client"
	"github.com/SENERGY-Platform/process-model-repository/lib/model"
	"log"
	"net/http"
)

func (this *Controller) RunMigrations() error {
	if !this.config.RunStartupMigration {
		return nil
	}
	if this.config.PermissionsUrl == "" || this.config.PermissionsUrl == "-" {
		log.Println("WARNING: skip migration without config.PermissionsUrl")
		return nil
	}
	log.Println("start migration")
	permsearchClient := permsearch.NewClient(this.config.PermissionsUrl)

	listOptions := model.ListOptions{Limit: 1000}
	for {
		ctx, _ := context.WithTimeout(context.Background(), TIMEOUT)
		batch, _, err := this.db.ListProcesses(ctx, listOptions)
		if err != nil {
			return err
		}
		listOptions.Offset = listOptions.Offset + listOptions.Limit
		if len(batch) == 0 {
			return nil
		}
		for _, process := range batch {
			_, err, code := this.perm.GetResource(client.InternalAdminToken, this.config.ProcessTopic, process.Id)
			if err != nil && code != http.StatusNotFound {
				return err
			}
			if code == http.StatusNotFound {
				legacyRights, err := permsearchClient.GetRights(client.InternalAdminToken, this.config.ProcessTopic, process.Id)
				if errors.Is(err, permsearchmodel.ErrNotFound) {
					log.Printf("process %v %v has no known permissions --> set default\n", process.Name, process.Id)
					_, err, _ = this.perm.SetPermission(client.InternalAdminToken, this.config.ProcessTopic, process.Id, client.ResourcePermissions{
						UserPermissions: map[string]client.PermissionsMap{
							process.Owner: {Read: true, Write: true, Execute: true, Administrate: true},
						},
					})
					if err != nil {
						return err
					}
					continue
				}
				if err != nil {
					return err
				}
				permissions := client.ResourcePermissions{
					UserPermissions:  map[string]client.PermissionsMap{},
					GroupPermissions: map[string]client.PermissionsMap{},
					RolePermissions:  map[string]client.PermissionsMap{},
				}

				for user, right := range legacyRights.UserRights {
					permissions.UserPermissions[user] = client.PermissionsMap{
						Read:         right.Read,
						Write:        right.Write,
						Execute:      right.Execute,
						Administrate: right.Administrate,
					}
				}

				//groups became rolls and keycloak groups are the new groups
				for roll, right := range legacyRights.GroupRights {
					permissions.RolePermissions[roll] = client.PermissionsMap{
						Read:         right.Read,
						Write:        right.Write,
						Execute:      right.Execute,
						Administrate: right.Administrate,
					}
				}

				log.Printf("set process %v %v permissions\n", process.Name, process.Id)
				_, err, _ = this.perm.SetPermission(client.InternalAdminToken, this.config.ProcessTopic, process.Id, permissions)
				if err != nil {
					return err
				}
			}
		}
	}
}
