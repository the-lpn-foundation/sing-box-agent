package sync

import (
	"github.com/oglenyaboss/sing-box-agent/internal/models"
)

// ChangeType represents the type of change to apply.
type ChangeType string

const (
	ChangeCreate ChangeType = "create"
	ChangeUpdate ChangeType = "update"
	ChangeDelete ChangeType = "delete"
)

// InboundChange represents a change to an inbound.
type InboundChange struct {
	Type     ChangeType
	Inbound  models.Inbound
	Previous *models.Inbound
}

// UserChange represents a change to a user.
type UserChange struct {
	Type     ChangeType
	User     models.User
	Previous *models.User
}

// DiffInbounds compares desired and current inbounds and returns changes.
func DiffInbounds(desired, current []models.Inbound) []InboundChange {
	var changes []InboundChange

	desiredMap := make(map[string]models.Inbound)
	for _, inbound := range desired {
		desiredMap[inbound.Tag] = inbound
	}

	currentMap := make(map[string]models.Inbound)
	for _, inbound := range current {
		currentMap[inbound.Tag] = inbound
	}

	for tag, desiredInbound := range desiredMap {
		if currentInbound, exists := currentMap[tag]; exists {
			if inboundsDiffer(desiredInbound, currentInbound) {
				changes = append(changes, InboundChange{
					Type:     ChangeUpdate,
					Inbound:  desiredInbound,
					Previous: &currentInbound,
				})
			}
		} else {
			changes = append(changes, InboundChange{
				Type:    ChangeCreate,
				Inbound: desiredInbound,
			})
		}
	}

	for tag, currentInbound := range currentMap {
		if _, exists := desiredMap[tag]; !exists {
			changes = append(changes, InboundChange{
				Type:     ChangeDelete,
				Inbound:  currentInbound,
				Previous: &currentInbound,
			})
		}
	}

	return changes
}

// DiffUsers compares desired and current users and returns changes.
func DiffUsers(desired, current []models.User) []UserChange {
	var changes []UserChange

	desiredMap := make(map[string]models.User)
	for _, user := range desired {
		desiredMap[user.SubID] = user
	}

	currentMap := make(map[string]models.User)
	for _, user := range current {
		currentMap[user.SubID] = user
	}

	for subID, desiredUser := range desiredMap {
		if currentUser, exists := currentMap[subID]; exists {
			if usersDiffer(desiredUser, currentUser) {
				changes = append(changes, UserChange{
					Type:     ChangeUpdate,
					User:     desiredUser,
					Previous: &currentUser,
				})
			}
		} else {
			changes = append(changes, UserChange{
				Type: ChangeCreate,
				User: desiredUser,
			})
		}
	}

	for subID, currentUser := range currentMap {
		if _, exists := desiredMap[subID]; !exists {
			changes = append(changes, UserChange{
				Type:     ChangeDelete,
				User:     currentUser,
				Previous: &currentUser,
			})
		}
	}

	return changes
}

// inboundsDiffer checks if two inbounds have different configurations.
func inboundsDiffer(a, b models.Inbound) bool {
	if a.Tag != b.Tag || a.Type != b.Type || a.Listen != b.Listen || a.Port != b.Port {
		return true
	}

	if len(a.Options) != len(b.Options) {
		return true
	}

	for k, v := range a.Options {
		if bv, exists := b.Options[k]; !exists || !interfaceEqual(v, bv) {
			return true
		}
	}

	return false
}

// usersDiffer checks if two users have different configurations.
func usersDiffer(a, b models.User) bool {
	if a.SubID != b.SubID || a.UUID != b.UUID || a.InboundTag != b.InboundTag ||
		a.Email != b.Email || a.Enabled != b.Enabled || a.Flow != b.Flow ||
		a.LimitIP != b.LimitIP || a.UploadLimit != b.UploadLimit || a.DownloadLimit != b.DownloadLimit {
		return true
	}

	if (a.ResetAt == nil) != (b.ResetAt == nil) {
		return true
	}

	if a.ResetAt != nil && b.ResetAt != nil && !a.ResetAt.Equal(*b.ResetAt) {
		return true
	}

	return false
}

// interfaceEqual compares two interface{} values for equality.
func interfaceEqual(a, b interface{}) bool {
	switch a := a.(type) {
	case map[string]interface{}:
		bMap, ok := b.(map[string]interface{})
		if !ok || len(a) != len(bMap) {
			return false
		}
		for k, v := range a {
			if bv, exists := bMap[k]; !exists || !interfaceEqual(v, bv) {
				return false
			}
		}
		return true
	case []interface{}:
		bSlice, ok := b.([]interface{})
		if !ok || len(a) != len(bSlice) {
			return false
		}
		for i, v := range a {
			if !interfaceEqual(v, bSlice[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}
