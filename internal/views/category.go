package views

import (
	"godiscourse/internal/model"
	"net/http"
	"time"
)

// CategoryView is the response body of a category
type CategoryView struct {
	Type        string    `json:"type"`
	CategoryID  string    `json:"category_id"`
	Name        string    `json:"name"`
	Alias       string    `json:"alias"`
	Description string    `json:"description"`
	TopicsCount int64     `json:"topics_count"`
	LastTopicID string    `json:"last_topic_id"`
	Position    int64     `json:"position"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func buildCategory(category *model.Category) (result CategoryView) {
	if category != nil {
		result = CategoryView{
			Type:        "category",
			CategoryID:  category.CategoryID,
			Name:        category.Name,
			Alias:       category.Alias,
			Description: category.Description,
			TopicsCount: category.TopicsCount,
			LastTopicID: category.LastTopicID.String,
			Position:    category.Position,
			CreatedAt:   category.CreatedAt,
			UpdatedAt:   category.UpdatedAt,
		}
	}
	return
}

// RenderCategory response a category
func RenderCategory(w http.ResponseWriter, r *http.Request, category *model.Category) {
	RenderResponse(w, r, buildCategory(category))
}

// RenderCategories response sevaral categories
func RenderCategories(w http.ResponseWriter, r *http.Request, categories []*model.Category) {
	views := make([]CategoryView, len(categories))
	for i, c := range categories {
		views[i] = buildCategory(c)
	}
	RenderResponse(w, r, views)
}
