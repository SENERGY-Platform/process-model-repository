/*
 * Copyright 2026 InfAI (CC SES)
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

package model

import "errors"

// ErrUsageUnavailable means the smart-service-repository could not tell whether smart services
// use a process model. Its text reaches the client, so wrap it only with static strings.
var ErrUsageUnavailable = errors.New("could not check whether smart services use the process model, nothing was deleted")

// ResourceRef names a smart service design that uses the resource and that the caller may read.
type ResourceRef struct {
	Id       string `json:"id"`
	DesignId string `json:"design_id"`
	Name     string `json:"name"`
}

// ResourceUsage is the smart-service-repository's answer: Releases and Instances count over all
// users, Readable lists only what the caller may read.
type ResourceUsage struct {
	Releases  int           `json:"releases"`
	Instances int           `json:"instances"`
	Readable  []ResourceRef `json:"readable"`
}

// InUseError refuses a deletion because smart services still use the process model. The text is
// static; the API builds the response body from Usage.
type InUseError struct {
	Usage ResourceUsage
}

func (e *InUseError) Error() string {
	return "process model is still used by smart services"
}

// InUseResponse is the 409 body of the delete.
type InUseResponse struct {
	Error string `json:"error"`
	ResourceUsage
}
