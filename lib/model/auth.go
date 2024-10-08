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

package model

import "github.com/SENERGY-Platform/permissions-v2/pkg/client"

type AuthAction string

const (
	READ         AuthAction = "r"
	WRITE        AuthAction = "w"
	EXECUTE      AuthAction = "x"
	ADMINISTRATE AuthAction = "a"
)

func (this AuthAction) String() string {
	return string(this)
}

func (this AuthAction) ToPermission() client.Permission {
	right := client.Read
	switch this {
	case READ:
		right = client.Read
	case WRITE:
		right = client.Write
	case ADMINISTRATE:
		right = client.Administrate
	case EXECUTE:
		right = client.Execute
	}
	return right
}
