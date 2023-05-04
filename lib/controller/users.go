package controller

import (
	"github.com/SENERGY-Platform/permission-search/lib/client"
	"github.com/SENERGY-Platform/permission-search/lib/model"
	"github.com/SENERGY-Platform/process-model-repository/lib/auth"
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
		options := client.ListOptions{
			QueryListCommons: model.QueryListCommons{
				Limit:    batchsize,
				Rights:   rights,
				SortBy:   "name",
				SortDesc: false,
			},
		}
		if lastElement.Id == "" {
			options.Offset = 0
		} else {
			options.After = &client.ListAfter{
				SortFieldValue: lastElement.Name,
				Id:             lastElement.Id,
			}
		}
		temp, err := client.List[[]PermSearchElement](this.permissionsearch, token.Jwt(), resource, options)
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

func contains(list []string, value string) bool {
	for _, element := range list {
		if element == value {
			return true
		}
	}
	return false
}
