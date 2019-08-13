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
	"github.com/SENERGY-Platform/process-model-repository/lib/model"
	"github.com/SmartEnergyPlatform/jwt-http-router"
)

type Controller interface {
	ReadProcess(jwt jwt_http_router.Jwt, id string, action model.AuthAction) (result model.Process, err error, errCode int)
	ReadAllPublicProcess() ([]model.Process, error, int)
	PublishProcessCreate(jwt jwt_http_router.Jwt, process model.Process) (model.Process, error, int)
	PublishProcessUpdate(jwt jwt_http_router.Jwt, id string, process model.Process) (model.Process, error, int)
	PublishProcessPublicUpdate(jwt jwt_http_router.Jwt, id string, public bool) (model.Process, error, int)
	PublishProcessDelete(jwt jwt_http_router.Jwt, id string) (error, int)
}
