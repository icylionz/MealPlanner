package layouts

import (
	"strings"

	"mealplanner/internal/models"
)

func navClass(active, key string) string {
	if active == key {
		return "app-nav-link active"
	}
	return "app-nav-link"
}

func Route(basePath, path string) string {
	if path == "" {
		return basePath
	}
	if basePath == "" || basePath == "/" {
		return path
	}
	if path == "/" {
		return basePath
	}
	return strings.TrimRight(basePath, "/") + path
}

func bodyClass(page models.AppPageData) string {
	if page.CurrentUser == nil {
		return "auth-body"
	}
	if page.CurrentUser.HouseholdID == nil {
		return "onboarding-body"
	}
	return "app-body"
}

func pageDescription(active string) string {
	switch active {
	case "plan":
		return "Shape the next stretch of meals and keep the week moving."
	case "grocery":
		return "Turn scheduled meals into a calm, shoppable market list."
	case "recipes":
		return "Build a reusable library of dishes, components, and yields."
	case "ingredients":
		return "Manage your canonical pantry data, aliases, and density rules."
	case "settings":
		return "Control household access, exports, and kitchen administration."
	default:
		return "Shared household planning for meals, groceries, and recipes."
	}
}

func currentUserLabel(user *models.CurrentUser) string {
	if user == nil {
		return ""
	}
	if trimmed := strings.TrimSpace(user.Username); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(user.Email)
}

func currentUserInitials(user *models.CurrentUser) string {
	label := currentUserLabel(user)
	if label == "" {
		return "MP"
	}
	parts := strings.FieldsFunc(label, func(r rune) bool {
		return r == ' ' || r == '.' || r == '_' || r == '-' || r == '@'
	})
	initials := make([]string, 0, 2)
	for _, part := range parts {
		if part == "" {
			continue
		}
		initials = append(initials, strings.ToUpper(part[:1]))
		if len(initials) == 2 {
			break
		}
	}
	if len(initials) == 0 {
		return strings.ToUpper(label[:1])
	}
	return strings.Join(initials, "")
}
