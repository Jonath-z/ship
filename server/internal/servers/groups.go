package servers

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Jonath-z/ship/server/migrations"
)

var (
	ErrEnvironmentNotFound = errors.New("environment was not found")
	ErrGroupNotFound       = errors.New("server group was not found")
	ErrAlreadyMember       = errors.New("server is already in this group")
	ErrNotMember           = errors.New("server is not in this group")
	ErrLastMemberInUse     = errors.New("cannot remove the last server from a group that services or accessories target")
)

type GroupResource struct {
	ID      string           `json:"id"`
	Name    string           `json:"name"`
	Members []ServerResource `json:"members"`
}

// Groups lists an environment's server groups with their member servers.
// Groups themselves are created implicitly when a service declares a role.
func (service *Service) Groups(ctx context.Context, projectID, environmentID string) ([]GroupResource, error) {
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return nil, err
	}
	var groups []migrations.ServerGroup
	if err := service.db.WithContext(ctx).Where("environment_id = ?", environmentID).
		Order("name ASC").Find(&groups).Error; err != nil {
		return nil, fmt.Errorf("list server groups: %w", err)
	}
	resources := make([]GroupResource, 0, len(groups))
	for _, group := range groups {
		var members []migrations.Server
		err := service.db.WithContext(ctx).
			Joins("JOIN server_group_memberships ON server_group_memberships.server_id = servers.id").
			Where("server_group_memberships.server_group_id = ?", group.ID).
			Order("servers.name ASC").Find(&members).Error
		if err != nil {
			return nil, fmt.Errorf("list group members: %w", err)
		}
		resource := GroupResource{ID: group.ID, Name: group.Name, Members: make([]ServerResource, 0, len(members))}
		for _, member := range members {
			resource.Members = append(resource.Members, response(member))
		}
		resources = append(resources, resource)
	}
	return resources, nil
}

// AddMember places a server into a role group (SH-045). The service targeting
// the role resolves to all member servers at render time.
func (service *Service) AddMember(ctx context.Context, requestContext RequestContext, projectID, environmentID, groupID, serverID string) error {
	group, err := service.findGroup(ctx, projectID, environmentID, groupID)
	if err != nil {
		return err
	}
	server, err := service.find(ctx, serverID)
	if err != nil {
		return err
	}
	membership := migrations.ServerGroupMembership{ServerGroupID: group.ID, ServerID: server.ID}
	if err := service.db.WithContext(ctx).Create(&membership).Error; err != nil {
		if uniqueViolation(err) {
			return ErrAlreadyMember
		}
		return fmt.Errorf("add group member: %w", err)
	}
	service.record(ctx, requestContext, "server.group.joined", server)
	return nil
}

// RemoveMember drops a server from a role group. Removing the last member of
// a group that services or accessories target fails validation.
func (service *Service) RemoveMember(ctx context.Context, requestContext RequestContext, projectID, environmentID, groupID, serverID string) error {
	group, err := service.findGroup(ctx, projectID, environmentID, groupID)
	if err != nil {
		return err
	}
	server, err := service.find(ctx, serverID)
	if err != nil {
		return err
	}
	var members int64
	if err := service.db.WithContext(ctx).Model(&migrations.ServerGroupMembership{}).
		Where("server_group_id = ?", group.ID).Count(&members).Error; err != nil {
		return fmt.Errorf("count group members: %w", err)
	}
	if members == 1 {
		inUse, err := service.groupInUse(ctx, group.ID)
		if err != nil {
			return err
		}
		if inUse {
			return ErrLastMemberInUse
		}
	}
	result := service.db.WithContext(ctx).
		Where("server_group_id = ? AND server_id = ?", group.ID, server.ID).
		Delete(&migrations.ServerGroupMembership{})
	if result.Error != nil {
		return fmt.Errorf("remove group member: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotMember
	}
	service.record(ctx, requestContext, "server.group.left", server)
	return nil
}

func (service *Service) groupInUse(ctx context.Context, groupID string) (bool, error) {
	var services int64
	if err := service.db.WithContext(ctx).Model(&migrations.Service{}).
		Where("server_group_id = ?", groupID).Count(&services).Error; err != nil {
		return false, fmt.Errorf("count group services: %w", err)
	}
	if services > 0 {
		return true, nil
	}
	var accessories int64
	if err := service.db.WithContext(ctx).Model(&migrations.Accessory{}).
		Where("server_group_id = ?", groupID).Count(&accessories).Error; err != nil {
		return false, fmt.Errorf("count group accessories: %w", err)
	}
	return accessories > 0, nil
}

func (service *Service) findGroup(ctx context.Context, projectID, environmentID, groupID string) (migrations.ServerGroup, error) {
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return migrations.ServerGroup{}, err
	}
	if uuid.Validate(groupID) != nil {
		return migrations.ServerGroup{}, ErrGroupNotFound
	}
	var group migrations.ServerGroup
	err := service.db.WithContext(ctx).First(&group, "id = ? AND environment_id = ?", groupID, environmentID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return migrations.ServerGroup{}, ErrGroupNotFound
	}
	if err != nil {
		return migrations.ServerGroup{}, fmt.Errorf("find server group: %w", err)
	}
	return group, nil
}

func (service *Service) requireEnvironment(ctx context.Context, projectID, environmentID string) error {
	if uuid.Validate(projectID) != nil || uuid.Validate(environmentID) != nil {
		return ErrEnvironmentNotFound
	}
	var count int64
	err := service.db.WithContext(ctx).Model(&migrations.Environment{}).
		Where("id = ? AND project_id = ?", environmentID, projectID).Count(&count).Error
	if err != nil {
		return fmt.Errorf("find environment: %w", err)
	}
	if count != 1 {
		return ErrEnvironmentNotFound
	}
	return nil
}
