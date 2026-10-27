package tests

import (
	"fmt"
	"time"

	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func tokenForTestUser(username, role string, duration time.Duration) (string, error) {
	user, err := storage.GetGlobalRepository().GetUserByUsername(username)
	if err != nil {
		return "", err
	}
	if user.Role != role {
		return "", fmt.Errorf("fixture role mismatch: %s", username)
	}
	return controlplane.GenerateAdminToken(user, duration)
}
