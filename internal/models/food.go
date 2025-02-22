package models

type Food struct {
	ID       int     `json:"id"`
	Name     string  `json:"name"`
	UnitType string  `json:"unitType"`
	BaseUnit string  `json:"baseUnit"`
	Density  float64 `json:"density,omitempty"`
	IsRecipe bool    `json:"isRecipe"`
	Recipe   *Recipe `json:"recipe,omitempty"`
}

type Recipe struct {
	Instructions  string        `json:"instructions,omitempty"`
	URL           string        `json:"url,omitempty"`
	YieldQuantity float64       `json:"yieldQuantity"`
	YieldUnit     string        `json:"yieldUnit"`
	Ingredients   []*RecipeItem `json:"ingredients"`
}

type RecipeItem struct {
	FoodID   int     `json:"foodId"`
	Food     *Food   `json:"food"`
	Quantity float64 `json:"quantity"`
	Unit     string  `json:"unit"`
}
