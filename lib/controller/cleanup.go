/*
 * Copyright 2025 InfAI (CC SES)
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
	"github.com/SENERGY-Platform/permissions-v2/pkg/client"
	"log"
	"time"
)

func (this *Controller) StartCleanupLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				start := time.Now()
				permissionsRemoved, processesRemoved, err := this.Cleanup()
				if err != nil {
					log.Printf("ERROR: while cleaning up process permissions: %v", err)
				} else {
					log.Printf("INFO: cleaned up process permissions in %v, permissions removed: %v, processes removed: %v", time.Now().Sub(start), permissionsRemoved, processesRemoved)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (this *Controller) Cleanup() (permissionsRemoved int, processesRemoved int, err error) {
	ids, err, _ := this.perm.AdminListResourceIds(client.InternalAdminToken, this.config.ProcessTopic, client.ListOptions{})
	if err != nil {
		return permissionsRemoved, processesRemoved, err
	}
	missingInDb, missingInPerm, err := this.db.CheckIdList(ids)
	if err != nil {
		return permissionsRemoved, processesRemoved, err
	}
	for _, id := range missingInDb {
		permissionsRemoved++
		err, _ = this.perm.RemoveResource(client.InternalAdminToken, this.config.ProcessTopic, id)
		if err != nil {
			return permissionsRemoved, processesRemoved, err
		}
	}
	for _, id := range missingInPerm {
		processesRemoved++
		ctx, _ := context.WithTimeout(context.Background(), TIMEOUT)
		err = this.db.DeleteProcess(ctx, id)
		if err != nil {
			return permissionsRemoved, processesRemoved, err
		}
	}
	return permissionsRemoved, processesRemoved, nil
}
