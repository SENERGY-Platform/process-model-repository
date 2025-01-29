package controller

import (
	"github.com/SENERGY-Platform/permissions-v2/pkg/client"
	"github.com/SENERGY-Platform/process-model-repository/lib/auth"
	"slices"
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
		err, _ = this.deleteProcess(id)
		if err != nil {
			return err
		}
	}
	for _, r := range userToDeleteFromProcessModels {
		delete(r.UserPermissions, userId)
		_, err, _ = this.perm.SetPermission(client.InternalAdminToken, this.config.ProcessTopic, r.Id, r.ResourcePermissions)
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

var ResourcesEffectedByUserDelete_BATCH_SIZE int64 = 1000

func containsOtherAdmin(m map[string]client.PermissionsMap, notThisKey string) bool {
	for k, v := range m {
		if k != notThisKey && v.Administrate {
			return true
		}
	}
	return false
}

func (this *Controller) ResourcesEffectedByUserDelete(token auth.Token, resource string) (deleteResourceIds []string, deleteUserFromResource []client.Resource, err error) {
	userid := token.GetUserId()
	err = this.iterateResource(token, resource, ResourcesEffectedByUserDelete_BATCH_SIZE, client.Administrate, func(element client.Resource) {
		if containsOtherAdmin(element.UserPermissions, userid) {
			deleteUserFromResource = append(deleteUserFromResource, element)
		} else {
			deleteResourceIds = append(deleteResourceIds, element.Id)
		}
	})
	if err != nil {
		return
	}

	err = this.iterateResource(token, resource, ResourcesEffectedByUserDelete_BATCH_SIZE, client.Read, func(element client.Resource) {
		if !slices.ContainsFunc(deleteUserFromResource, func(resource client.Resource) bool {
			return resource.Id == element.Id
		}) {
			deleteUserFromResource = append(deleteUserFromResource, element)
		}
	})
	if err != nil {
		return
	}
	err = this.iterateResource(token, resource, ResourcesEffectedByUserDelete_BATCH_SIZE, client.Write, func(element client.Resource) {
		if !slices.ContainsFunc(deleteUserFromResource, func(resource client.Resource) bool {
			return resource.Id == element.Id
		}) {
			deleteUserFromResource = append(deleteUserFromResource, element)
		}
	})
	if err != nil {
		return
	}
	err = this.iterateResource(token, resource, ResourcesEffectedByUserDelete_BATCH_SIZE, client.Execute, func(element client.Resource) {
		if !slices.ContainsFunc(deleteUserFromResource, func(resource client.Resource) bool {
			return resource.Id == element.Id
		}) {
			deleteUserFromResource = append(deleteUserFromResource, element)
		}
	})
	if err != nil {
		return
	}
	return deleteResourceIds, deleteUserFromResource, err
}

func (this *Controller) iterateResource(token auth.Token, resource string, batchsize int64, rights client.Permission, handler func(element client.Resource)) (err error) {
	lastCount := batchsize
	var offset int64 = 0
	for lastCount == batchsize {
		options := client.ListOptions{
			Limit:  batchsize,
			Offset: offset,
		}
		offset += batchsize
		ids, err, _ := this.perm.ListAccessibleResourceIds(token.Jwt(), resource, options, rights)
		if err != nil {
			return err
		}
		lastCount = int64(len(ids))
		for _, id := range ids {
			element, err, _ := this.perm.GetResource(client.InternalAdminToken, resource, id)
			if err != nil {
				return err
			}
			handler(element)
		}
	}
	return err
}
