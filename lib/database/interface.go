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

package database

import (
	"context"
	"github.com/SENERGY-Platform/process-model-repository/lib/model"
)

type Database interface {
	ReadProcess(ctx context.Context, id string) (result model.Process, exists bool, err error)
	ReadAllPublicProcesses(ctx context.Context) ([]model.Process, error)
	SetProcess(ctx context.Context, process model.Process) error
	DeleteProcess(ctx context.Context, id string) error
	ListProcesses(ctx context.Context, options model.ListOptions) ([]model.Process, int64, error)
	CheckIdList(ids []string) (missingInDb []string, missingInInput []string, err error)
}
