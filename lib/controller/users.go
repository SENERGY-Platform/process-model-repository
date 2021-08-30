package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/SENERGY-Platform/process-model-repository/lib/controller/auth"
	"log"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
)

func (this *Controller) HandleUserDelete(userId string) error {
	processModelResource := "processmodel"
	token, err := auth.CreateToken("process-model-repo", userId)
	if err != nil {
		return err
	}
	processModelsToDelete, userToDeleteFromProcessModels, err := this.ResourcesEffectedByUserDelete(token, processModelResource)
	if err != nil {
		return err
	}
	for _, id := range processModelsToDelete {
		err = this.producer.PublishProcessDelete(id, userId)
		if err != nil {
			return err
		}
	}
	for _, id := range userToDeleteFromProcessModels {
		err = this.producer.PublishDeleteUserRights(processModelResource, id, userId)
		if err != nil {
			return err
		}
	}
	return nil
}

type PermSearchElement struct {
	Id                string            `json:"id"`
	Name              string            `json:"name"`
	Shared            bool              `json:"shared"`
	Creator           string            `json:"creator"`
	PermissionHolders PermissionHolders `json:"permission_holders"`
}

type PermissionHolders struct {
	AdminUsers   []string `json:"admin_users"`
	ReadUsers    []string `json:"read_users"`
	WriteUsers   []string `json:"write_users"`
	ExecuteUsers []string `json:"execute_users"`
}

var ResourcesEffectedByUserDelete_BATCH_SIZE = 1000

func (this *Controller) ResourcesEffectedByUserDelete(token auth.Token, resource string) (deleteResourceIds []string, deleteUserFromResourceIds []string, err error) {
	err = this.iterateResource(token, resource, ResourcesEffectedByUserDelete_BATCH_SIZE, "a", func(element PermSearchElement) {
		if len(element.PermissionHolders.AdminUsers) > 1 {
			deleteUserFromResourceIds = append(deleteUserFromResourceIds, element.Id)
		} else {
			deleteResourceIds = append(deleteResourceIds, element.Id)
		}
	})
	if err != nil {
		return
	}
	userid := token.GetUserId()
	err = this.iterateResource(token, resource, ResourcesEffectedByUserDelete_BATCH_SIZE, "r", func(element PermSearchElement) {
		if !contains(element.PermissionHolders.AdminUsers, userid) {
			deleteUserFromResourceIds = append(deleteUserFromResourceIds, element.Id)
		}
	})
	if err != nil {
		return
	}
	err = this.iterateResource(token, resource, ResourcesEffectedByUserDelete_BATCH_SIZE, "w", func(element PermSearchElement) {
		if !contains(element.PermissionHolders.AdminUsers, userid) &&
			!contains(element.PermissionHolders.ReadUsers, userid) {
			deleteUserFromResourceIds = append(deleteUserFromResourceIds, element.Id)
		}
	})
	if err != nil {
		return
	}
	err = this.iterateResource(token, resource, ResourcesEffectedByUserDelete_BATCH_SIZE, "x", func(element PermSearchElement) {
		if !contains(element.PermissionHolders.AdminUsers, userid) &&
			!contains(element.PermissionHolders.ReadUsers, userid) &&
			!contains(element.PermissionHolders.WriteUsers, userid) {
			deleteUserFromResourceIds = append(deleteUserFromResourceIds, element.Id)
		}
	})
	if err != nil {
		return
	}
	return deleteResourceIds, deleteUserFromResourceIds, err
}

func (this *Controller) iterateResource(token auth.Token, resource string, batchsize int, rights string, handler func(element PermSearchElement)) (err error) {
	lastCount := batchsize
	lastElement := PermSearchElement{}
	for lastCount == batchsize {
		query := url.Values{}
		query.Add("limit", strconv.Itoa(batchsize))
		query.Add("sort", "name.asc")
		query.Add("rights", rights)
		if lastElement.Id == "" {
			query.Add("offset", "0")
		} else {
			name, err := json.Marshal(lastElement.Name)
			if err != nil {
				return err
			}
			query.Add("after.sort_field_value", string(name))
			query.Add("after.id", lastElement.Id)
		}
		temp := []PermSearchElement{}
		err = this.queryResourceInPermsearch(token, resource, query, &temp)
		if err != nil {
			return err
		}
		lastCount = len(temp)
		if lastCount > 0 {
			lastElement = temp[lastCount-1]
		}
		for _, element := range temp {
			handler(element)
		}
	}
	return err
}

func (this *Controller) queryResourceInPermsearch(token auth.Token, resource string, query url.Values, result interface{}) (err error) {
	req, err := http.NewRequest("GET", this.config.PermissionsUrl+"/v3/resources/"+resource+"?"+query.Encode(), nil)
	if err != nil {
		debug.PrintStack()
		return err
	}
	req.Header.Set("Authorization", token.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		debug.PrintStack()
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		err = errors.New(buf.String())
		log.Println("ERROR: queryResourceInPermsearch()", resource, resp.StatusCode, err)
		debug.PrintStack()
		return err
	}
	err = json.NewDecoder(resp.Body).Decode(result)
	return
}

func contains(list []string, value string) bool {
	for _, element := range list {
		if element == value {
			return true
		}
	}
	return false
}
