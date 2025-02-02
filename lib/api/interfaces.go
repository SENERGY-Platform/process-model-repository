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
	"github.com/SENERGY-Platform/process-model-repository/lib/auth"
	"github.com/SENERGY-Platform/process-model-repository/lib/model"
)

type Controller interface {
	ReadProcess(token auth.Token, id string, action model.AuthAction) (result model.Process, err error, errCode int)
	ListProcesses(token auth.Token, options model.ListOptions) ([]model.Process, int64, error, int)
	ReadAllPublicProcess() ([]model.Process, error, int)
	CreateProcess(token auth.Token, process model.Process) (model.Process, error, int)
	UpdateProcess(token auth.Token, id string, process model.Process) (model.Process, error, int)
	UpdateProcessPublic(token auth.Token, id string, public model.PublicCommand) (model.Process, error, int)
	DeleteProcess(token auth.Token, id string) (error, int)
}
